package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/ezz-no/orche/channel"
	"github.com/ezz-no/orche/executor"
	"github.com/ezz-no/orche/graph"
	"github.com/ezz-no/orche/node"
)

var (
	configFile = flag.String("f", "", "Path to workflow config file (YAML/JSON)")
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
		log.Fatal(err)
	}
}

func printHelp() {
	fmt.Printf(`orche - Workflow Orchestration System

Usage:
  orche -f <config-file>    Run workflow from config file
  orche -h                  Show this help message

Examples:
  orche -f workflow.yaml    Run workflow from YAML file
  orche -f workflow.json    Run workflow from JSON file

Config File Format (YAML):
  nodes:
    - id: fetch
      type: process
      binary: curl
      args: ["-s", "http://example.com"]
    - id: parse
      type: func
      handler: parseJSON
  connections:
    - from: fetch
      to: parse

`)
}

func run(ctx context.Context, configFile string) error {
	fmt.Printf("Loading config from: %s\n", configFile)

	g := graph.NewGraphBuilder().
		Node("generator", node.NewGeneratorNode("generator", "data-generator", func(ctx context.Context) (map[string][]byte, error) {
			return map[string][]byte{
				"data": []byte(`{"message": "hello world"}`),
			}, nil
		}, []node.Port{
			{Name: "data", Type: "json", Required: true},
		})).
		Node("transform", node.NewFuncNode("transform", "data-transform", func(ctx context.Context, inputs map[string][]byte) (map[string][]byte, error) {
			data := inputs["data"]
			fmt.Printf("Transform got input: %s\n", string(data))
			return map[string][]byte{
				"result": append(data, []byte(" - transformed")...),
			}, nil
		}, []node.Port{
			{Name: "data", Type: "json", Required: true},
			{Name: "result", Type: "text", Required: true},
		})).
		Node("output", node.NewFuncNode("output", "stdout-output", func(ctx context.Context, inputs map[string][]byte) (map[string][]byte, error) {
			if result, ok := inputs["result"]; ok {
				fmt.Printf("Output: %s\n", string(result))
			} else {
				fmt.Printf("Output: no data received, inputs: %v\n", inputs)
			}
			return nil, nil
		}, []node.Port{
			{Name: "result", Type: "text", Required: true},
		})).
		ConnectWithPorts("generator", "transform", "data", "data").
		ConnectWithPorts("transform", "output", "result", "result").
		Build()

	chMgr := graph.NewChannelManager()
	ch, _ := chMgr.GetOrCreate("default", channel.ChannelMemory)
	chMgr.Register("default", ch)

	exec := executor.NewExecutor(g, chMgr, nil)
	exec.SetConfig(executor.ExecConfig{
		Parallelism: 2,
		RetryCount:  2,
		ErrorMode:   "stop",
	})

	if err := exec.Execute(ctx); err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	fmt.Println("Execution completed successfully")
	return nil
}
