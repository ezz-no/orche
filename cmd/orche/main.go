package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"

	"github.com/ezz-no/orche/channel"
	"github.com/ezz-no/orche/config"
	"github.com/ezz-no/orche/executor"
	"github.com/ezz-no/orche/graph"
	"github.com/ezz-no/orche/node"
)

var (
	configFile = flag.String("f", "", "Path to workflow config file (JSON)")
	showHelp   = flag.Bool("h", false, "Show help message")
)

func main() {
	flag.Parse()

	if *showHelp || *configFile == "" {
		printHelp()
		os.Exit(0)
	}

	ctx := context.Background()
	if err := run(ctx, *configFile); err != nil {
		fmt.Fprintln(os.Stderr, formatError(err))
		os.Exit(1)
	}
}

func run(ctx context.Context, configFile string) error {
	cfg, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("Workflow: %s\n", cfg.Name)
	if cfg.Description != "" {
		fmt.Printf("Description: %s\n", cfg.Description)
	}

	builder := graph.NewGraphBuilder()

	handlerRegistry := map[string]func(context.Context, map[string][]byte) (map[string][]byte, error){
		"transform": func(ctx context.Context, inputs map[string][]byte) (map[string][]byte, error) {
			data := inputs["data"]
			return map[string][]byte{
				"result": append(data, []byte(" - transformed")...),
			}, nil
		},
		"output": func(ctx context.Context, inputs map[string][]byte) (map[string][]byte, error) {
			if result, ok := inputs["result"]; ok {
				fmt.Printf("Output: %s\n", string(result))
			}
			return nil, nil
		},
		"extractImages": func(ctx context.Context, inputs map[string][]byte) (map[string][]byte, error) {
			html := inputs["html"]
			var urls []string

			re := regexp.MustCompile(`<img[^>]+src=["']([^"']+)["']`)
			matches := re.FindAllStringSubmatch(string(html), -1)
			for _, match := range matches {
				if len(match) > 1 {
					urls = append(urls, match[1])
				}
			}

			re2 := regexp.MustCompile(`url\(["']([^"']+)["']\)`)
			matches2 := re2.FindAllStringSubmatch(string(html), -1)
			for _, match := range matches2 {
				if len(match) > 1 {
					urls = append(urls, match[1])
				}
			}

			if len(urls) == 0 {
				return map[string][]byte{"images": []byte("[]")}, nil
			}

			out, _ := json.Marshal(urls)
			return map[string][]byte{"images": out}, nil
		},
		"printImages": func(ctx context.Context, inputs map[string][]byte) (map[string][]byte, error) {
			imagesData := inputs["images"]
			var urls []string
			json.Unmarshal(imagesData, &urls)

			fmt.Printf("Found %d image(s):\n", len(urls))
			for i, url := range urls {
				fmt.Printf("  %d. %s\n", i+1, url)
			}
			return nil, nil
		},
	}

	for _, n := range cfg.Nodes {
		switch n.Type {
		case "generator":
			var output map[string][]byte
			if len(n.Output) > 0 {
				output = make(map[string][]byte)
				for _, o := range n.Output {
					output[o.Name] = []byte(o.Value)
				}
			}
			if output == nil {
				output = map[string][]byte{"data": []byte("{}")}
			}
			builder.Node(n.ID, node.NewGeneratorNode(n.ID, n.ID, func(ctx context.Context) (map[string][]byte, error) {
				return output, nil
			}, []node.Port{{Name: "data", Type: "json", Required: true}}))

		case "func":
			if fn, ok := handlerRegistry[n.Handler]; ok {
				builder.Node(n.ID, node.NewFuncNode(n.ID, n.Handler, fn, []node.Port{
					{Name: "data", Type: "json", Required: true},
					{Name: "result", Type: "text", Required: true},
				}))
			} else {
				log.Printf("Warning: handler %s not found, skipping node %s", n.Handler, n.ID)
			}

		case "process":
			builder.Node(n.ID, node.NewProcessNode(n.ID, n.ID, n.Binary, n.Args))

		case "remote":
			builder.Node(n.ID, node.NewRemoteNode(n.ID, n.ID, n.Endpoint, nil, nil))
		}
	}

	for _, c := range cfg.Connections {
		fromPort := c.FromPort
		toPort := c.ToPort
		if fromPort == "" {
			fromPort = "default"
		}
		if toPort == "" {
			toPort = "default"
		}
		builder.ConnectWithPorts(c.From, c.To, fromPort, toPort)
	}

	g := builder.Build()

	chMgr := graph.NewChannelManager()
	ch, _ := chMgr.GetOrCreate("default", channel.ChannelMemory)
	chMgr.Register("default", ch)

	exec := executor.NewExecutor(g, chMgr, nil)
	execCfg := executor.DefaultExecConfig
	if cfg.Config.Parallelism > 0 {
		execCfg.Parallelism = cfg.Config.Parallelism
	}
	if cfg.Config.RetryCount > 0 {
		execCfg.RetryCount = cfg.Config.RetryCount
	}
	if cfg.Config.ErrorMode != "" {
		execCfg.ErrorMode = cfg.Config.ErrorMode
	}
	exec.SetConfig(execCfg)

	if err := exec.Execute(ctx); err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	fmt.Println("Execution completed successfully")
	return nil
}
