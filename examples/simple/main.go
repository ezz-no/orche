package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ezz-no/orche/channel"
	"github.com/ezz-no/orche/executor"
	"github.com/ezz-no/orche/graph"
	"github.com/ezz-no/orche/node"
)

func main() {
	ctx := context.Background()

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
		log.Fatalf("Execution failed: %v", err)
	}

	fmt.Println("Execution completed successfully")
}
