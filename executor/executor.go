package executor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ezz-no/orche/channel"
	"github.com/ezz-no/orche/graph"
	"github.com/ezz-no/orche/node"
	"github.com/ezz-no/orche/state"
)

type ExecConfig struct {
	Parallelism int
	RetryCount  int
	RetryDelay  time.Duration
	Timeout     time.Duration
	ErrorMode   string
}

var DefaultExecConfig = ExecConfig{
	Parallelism: 4,
	RetryCount:  3,
	RetryDelay:  time.Second,
	Timeout:     5 * time.Minute,
	ErrorMode:   "stop",
}

type Execution = state.Execution

type StateStore interface {
	SaveExecution(exec *state.Execution) error
	GetExecution(id string) (*state.Execution, error)
	SaveNodeState(nodeID string, outputs map[string][]byte) error
	GetNodeState(nodeID string) (map[string][]byte, error)
	SaveGraphState(graph *state.Graph) error
	GetGraphState() (*state.Graph, error)
	Clear() error
}

type Executor struct {
	graph       *graph.Graph
	channels    *graph.ChannelManager
	state       StateStore
	config      ExecConfig
	executions  map[string]*state.Execution
	mu          sync.RWMutex
	parallelism int
}

func NewExecutor(g *graph.Graph, chMgr *graph.ChannelManager, state StateStore) *Executor {
	return &Executor{
		graph:       g,
		channels:    chMgr,
		state:       state,
		config:      DefaultExecConfig,
		executions:  make(map[string]*Execution),
		parallelism: DefaultExecConfig.Parallelism,
	}
}

func (e *Executor) SetConfig(cfg ExecConfig) {
	e.config = cfg
	e.parallelism = cfg.Parallelism
}

func (e *Executor) Execute(ctx context.Context) error {
	if err := e.graph.Validate(); err != nil {
		return fmt.Errorf("invalid graph: %w", err)
	}

	exec := &Execution{
		ID:        fmt.Sprintf("exec-%d", time.Now().Unix()),
		Graph:     e.graph,
		Status:    "running",
		NodeCount: len(e.graph.Nodes()),
		StartTime: time.Now(),
	}

	e.mu.Lock()
	e.executions[exec.ID] = exec
	e.mu.Unlock()

	defer func() {
		exec.EndTime = time.Now()
		if err := recover(); err != nil {
			exec.Status = "failed"
			exec.Error = fmt.Errorf("panic: %v", err)
		}
	}()

	return e.executeGraph(ctx, exec)
}

func (e *Executor) executeGraph(ctx context.Context, exec *Execution) error {
	sorted, err := e.graph.TopologicalSort()
	if err != nil {
		return err
	}

	nodeStates := make(map[string]*nodeState)
	for id := range e.graph.Nodes() {
		nodeStates[id] = &nodeState{
			id:     id,
			status: node.StatusPending,
		}
	}

	readyCh := make(chan string, len(sorted))
	var wg sync.WaitGroup
	nodeCh := make(chan string, len(sorted))

	for _, id := range sorted {
		nodeCh <- id
	}
	close(nodeCh)

	var failed atomic.Bool
	var mu sync.Mutex
	var startIdx int
	sem := make(chan struct{}, e.parallelism)

	for id := range nodeCh {
		if failed.Load() && e.config.ErrorMode == "stop" {
			nodeStates[id].status = node.StatusSkipped
			continue
		}

		wg.Add(1)
		go func(nodeID string, idx int) {
			defer wg.Done()

			for {
				inputsReady := true
				for _, edge := range e.graph.GetIncomingEdges(nodeID) {
					fromState := nodeStates[edge.From]
					if fromState.outputs == nil {
						inputsReady = false
						break
					}
				}
				if inputsReady {
					break
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Millisecond):
				}
			}

			sem <- struct{}{}
			defer func() { <-sem }()

			if failed.Load() && e.config.ErrorMode == "stop" {
				nodeStates[nodeID].status = node.StatusSkipped
				return
			}

			ctx, cancel := context.WithTimeout(ctx, e.config.Timeout)
			defer cancel()

			err := e.executeNode(ctx, nodeID, nodeStates)
			if err != nil {
				mu.Lock()
				switch e.config.ErrorMode {
				case "stop":
					failed.Store(true)
				case "retry":
					for i := 0; i < e.config.RetryCount; i++ {
						time.Sleep(e.config.RetryDelay)
						if err := e.executeNode(ctx, nodeID, nodeStates); err == nil {
							break
						}
					}
				}
				mu.Unlock()
			}

			readyCh <- nodeID
		}(id, startIdx)
		startIdx++
	}

	wg.Wait()

	if failed.Load() {
		exec.Status = "failed"
		exec.Error = fmt.Errorf("execution failed")
		return exec.Error
	}

	exec.Status = "completed"
	return nil
}

type nodeState struct {
	id      string
	status  node.NodeStatus
	outputs map[string][]byte
	err     error
	retry   int
}

func (e *Executor) executeNode(ctx context.Context, nodeID string, states map[string]*nodeState) error {
	n := e.graph.Node(nodeID)
	if n == nil {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	states[nodeID].status = node.StatusRunning

	inputs := make(map[string][]byte)
	for _, edge := range e.graph.GetIncomingEdges(nodeID) {
		fromState := states[edge.From]
		if fromState.outputs != nil {
			if data, ok := fromState.outputs[edge.FromPort]; ok {
				inputs[edge.ToPort] = data
			}
		}
	}

	outputs, err := n.Run(ctx, inputs)
	if err != nil {
		states[nodeID].status = node.StatusFailed
		states[nodeID].err = err
		return err
	}

	states[nodeID].status = node.StatusCompleted
	states[nodeID].outputs = outputs

	for _, edge := range e.graph.GetOutgoingEdges(nodeID) {
		if data, ok := outputs[edge.FromPort]; ok {
			ch, err := e.channels.GetOrCreate(edge.From, channel.ChannelMemory)
			if err == nil {
				ch.Put(ctx, edge.From+"."+edge.FromPort, data)
			}
		}
	}

	if e.state != nil {
		e.state.SaveNodeState(nodeID, outputs)
	}

	return nil
}

func (e *Executor) GetExecution(id string) *Execution {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.executions[id]
}

func (e *Executor) ListExecutions() []*Execution {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]*Execution, 0, len(e.executions))
	for _, exec := range e.executions {
		result = append(result, exec)
	}
	return result
}
