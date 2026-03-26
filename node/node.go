package node

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type (
	NodeType   int
	NodeStatus int
)

const (
	NodeTypeFunc NodeType = iota
	NodeTypeProcess
	NodeTypeRemote
	NodeTypeGenerator
)

const (
	StatusPending NodeStatus = iota
	StatusRunning
	StatusCompleted
	StatusFailed
	StatusSkipped
)

type Port struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

type NodeConfig struct {
	RetryCount   int           `json:"retry_count"`
	RetryDelay   time.Duration `json:"retry_delay"`
	Timeout      time.Duration `json:"timeout"`
	ErrorHandler string        `json:"error_handler"`
}

var DefaultNodeConfig = NodeConfig{
	RetryCount:   3,
	RetryDelay:   time.Second,
	Timeout:      5 * time.Minute,
	ErrorHandler: "retry",
}

type Node interface {
	ID() string
	Type() NodeType
	Run(ctx context.Context, inputs map[string][]byte) (map[string][]byte, error)
	InputPorts() []Port
	OutputPorts() []Port
}

type FuncNode struct {
	id     string
	name   string
	ports  []Port
	fn     func(context.Context, map[string][]byte) (map[string][]byte, error)
	config NodeConfig
}

func NewFuncNode(id string, name string, fn func(context.Context, map[string][]byte) (map[string][]byte, error), ports []Port) *FuncNode {
	return &FuncNode{
		id:     id,
		name:   name,
		ports:  ports,
		fn:     fn,
		config: DefaultNodeConfig,
	}
}

func (n *FuncNode) ID() string               { return n.id }
func (n *FuncNode) Type() NodeType           { return NodeTypeFunc }
func (n *FuncNode) InputPorts() []Port       { return n.ports }
func (n *FuncNode) OutputPorts() []Port      { return n.ports }
func (n *FuncNode) Config() NodeConfig       { return n.config }
func (n *FuncNode) SetConfig(cfg NodeConfig) { n.config = cfg }

func (n *FuncNode) Run(ctx context.Context, inputs map[string][]byte) (map[string][]byte, error) {
	return n.fn(ctx, inputs)
}

type ProcessNode struct {
	id      string
	name    string
	binary  string
	args    []string
	workdir string
	env     map[string]string
	stdin   bool
	ports   []Port
	config  NodeConfig
}

func NewProcessNode(id string, name string, binary string, args []string) *ProcessNode {
	return &ProcessNode{
		id:     id,
		name:   name,
		binary: binary,
		args:   args,
		env:    make(map[string]string),
		ports:  []Port{{Name: "stdin", Type: "binary", Required: false}},
		config: DefaultNodeConfig,
	}
}

func (n *ProcessNode) ID() string         { return n.id }
func (n *ProcessNode) Type() NodeType     { return NodeTypeProcess }
func (n *ProcessNode) InputPorts() []Port { return n.ports }
func (n *ProcessNode) OutputPorts() []Port {
	return []Port{{Name: "stdout", Type: "binary", Required: true}, {Name: "stderr", Type: "binary", Required: false}}
}
func (n *ProcessNode) Config() NodeConfig       { return n.config }
func (n *ProcessNode) SetConfig(cfg NodeConfig) { n.config = cfg }

func (n *ProcessNode) Run(ctx context.Context, inputs map[string][]byte) (map[string][]byte, error) {
	cmd := exec.CommandContext(ctx, n.binary, n.args...)
	if n.workdir != "" {
		cmd.Dir = n.workdir
	}
	for k, v := range n.env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	if stdin, ok := inputs["stdin"]; ok && n.stdin {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	stdout, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return map[string][]byte{
				"stdout": stdout,
				"stderr": ee.Stderr,
			}, err
		}
		return nil, err
	}

	return map[string][]byte{
		"stdout": stdout,
	}, nil
}

type RemoteNode struct {
	id       string
	name     string
	endpoint string
	ports    []Port
	config   NodeConfig
	client   RemoteClient
}

type RemoteClient interface {
	Call(ctx context.Context, method string, inputs map[string][]byte) (map[string][]byte, error)
}

func NewRemoteNode(id string, name string, endpoint string, ports []Port, client RemoteClient) *RemoteNode {
	return &RemoteNode{
		id:       id,
		name:     name,
		endpoint: endpoint,
		ports:    ports,
		client:   client,
		config:   DefaultNodeConfig,
	}
}

func (n *RemoteNode) ID() string               { return n.id }
func (n *RemoteNode) Type() NodeType           { return NodeTypeRemote }
func (n *RemoteNode) InputPorts() []Port       { return n.ports }
func (n *RemoteNode) OutputPorts() []Port      { return n.ports }
func (n *RemoteNode) Config() NodeConfig       { return n.config }
func (n *RemoteNode) SetConfig(cfg NodeConfig) { n.config = cfg }

func (n *RemoteNode) Run(ctx context.Context, inputs map[string][]byte) (map[string][]byte, error) {
	if n.client == nil {
		return nil, fmt.Errorf("remote client not configured")
	}
	return n.client.Call(ctx, n.name, inputs)
}

type GeneratorNode struct {
	id    string
	name  string
	gen   func(context.Context) (map[string][]byte, error)
	ports []Port
}

func NewGeneratorNode(id string, name string, gen func(context.Context) (map[string][]byte, error), ports []Port) *GeneratorNode {
	return &GeneratorNode{
		id:    id,
		name:  name,
		gen:   gen,
		ports: ports,
	}
}

func (n *GeneratorNode) ID() string          { return n.id }
func (n *GeneratorNode) Type() NodeType      { return NodeTypeGenerator }
func (n *GeneratorNode) InputPorts() []Port  { return nil }
func (n *GeneratorNode) OutputPorts() []Port { return n.ports }

func (n *GeneratorNode) Run(ctx context.Context, inputs map[string][]byte) (map[string][]byte, error) {
	return n.gen(ctx)
}

type SerializationType int

const (
	SerializationJSON SerializationType = iota
	SerializationText
	SerializationBinary
)

func DetectSerialization(data []byte) SerializationType {
	var j any
	if json.Unmarshal(data, &j) == nil {
		return SerializationJSON
	}
	return SerializationText
}

func Serialize(data any, stype SerializationType) ([]byte, error) {
	switch stype {
	case SerializationJSON:
		return json.Marshal(data)
	case SerializationText:
		return []byte(fmt.Sprintf("%v", data)), nil
	default:
		if b, ok := data.([]byte); ok {
			return b, nil
		}
		return nil, fmt.Errorf("cannot serialize type %T to []byte", data)
	}
}
