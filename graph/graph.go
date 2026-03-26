package graph

import (
	"fmt"
	"strings"

	"github.com/ezz-no/orche/channel"
	"github.com/ezz-no/orche/node"
)

type Edge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	FromPort string `json:"from_port"`
	ToPort   string `json:"to_port"`
}

type Graph struct {
	nodes    map[string]node.Node
	edges    []Edge
	metadata map[string]string
}

func NewGraph() *Graph {
	return &Graph{
		nodes:    make(map[string]node.Node),
		edges:    make([]Edge, 0),
		metadata: make(map[string]string),
	}
}

func (g *Graph) AddNode(n node.Node) *Graph {
	g.nodes[n.ID()] = n
	return g
}

func (g *Graph) Node(id string) node.Node {
	return g.nodes[id]
}

func (g *Graph) Connect(from, to string) *Graph {
	return g.ConnectWithPorts(from, to, "default", "default")
}

func (g *Graph) ConnectWithPorts(from, to, fromPort, toPort string) *Graph {
	g.edges = append(g.edges, Edge{
		From:     from,
		To:       to,
		FromPort: fromPort,
		ToPort:   toPort,
	})
	return g
}

func (g *Graph) Edges() []Edge {
	return g.edges
}

func (g *Graph) Nodes() map[string]node.Node {
	return g.nodes
}

func (g *Graph) SetMetadata(key, value string) *Graph {
	g.metadata[key] = value
	return g
}

func (g *Graph) GetMetadata(key string) string {
	return g.metadata[key]
}

func (g *Graph) TopologicalSort() ([]string, error) {
	inDegree := make(map[string]int)
	for id := range g.nodes {
		inDegree[id] = 0
	}

	for _, e := range g.edges {
		inDegree[e.To]++
	}

	queue := make([]string, 0)
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	result := make([]string, 0, len(g.nodes))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		for _, e := range g.edges {
			if e.From == current {
				inDegree[e.To]--
				if inDegree[e.To] == 0 {
					queue = append(queue, e.To)
				}
			}
		}
	}

	if len(result) != len(g.nodes) {
		return nil, fmt.Errorf("graph contains a cycle")
	}

	return result, nil
}

func (g *Graph) GetOutgoingEdges(nodeID string) []Edge {
	var edges []Edge
	for _, e := range g.edges {
		if e.From == nodeID {
			edges = append(edges, e)
		}
	}
	return edges
}

func (g *Graph) GetIncomingEdges(nodeID string) []Edge {
	var edges []Edge
	for _, e := range g.edges {
		if e.To == nodeID {
			edges = append(edges, e)
		}
	}
	return edges
}

func (g *Graph) Validate() error {
	for _, e := range g.edges {
		if _, ok := g.nodes[e.From]; !ok {
			return fmt.Errorf("node not found: %s", e.From)
		}
		if _, ok := g.nodes[e.To]; !ok {
			return fmt.Errorf("node not found: %s", e.To)
		}
	}
	return nil
}

func (g *Graph) String() string {
	var sb strings.Builder
	sb.WriteString("Graph:\n")
	for id, n := range g.nodes {
		sb.WriteString(fmt.Sprintf("  Node: %s (%v)\n", id, n.Type()))
	}
	for _, e := range g.edges {
		sb.WriteString(fmt.Sprintf("  Edge: %s:%s -> %s:%s\n", e.From, e.FromPort, e.To, e.ToPort))
	}
	return sb.String()
}

type GraphBuilder struct {
	graph *Graph
}

func NewGraphBuilder() *GraphBuilder {
	return &GraphBuilder{
		graph: NewGraph(),
	}
}

func (b *GraphBuilder) Node(id string, n node.Node) *GraphBuilder {
	b.graph.AddNode(n)
	return b
}

func (b *GraphBuilder) Connect(from, to string) *GraphBuilder {
	b.graph.Connect(from, to)
	return b
}

func (b *GraphBuilder) ConnectWithPorts(from, to, fromPort, toPort string) *GraphBuilder {
	b.graph.ConnectWithPorts(from, to, fromPort, toPort)
	return b
}

func (b *GraphBuilder) Build() *Graph {
	return b.graph
}

type ChannelManager struct {
	channels map[string]channel.Channel
	defaults map[string]channel.ChannelType
}

func NewChannelManager() *ChannelManager {
	return &ChannelManager{
		channels: make(map[string]channel.Channel),
		defaults: make(map[string]channel.ChannelType),
	}
}

func (m *ChannelManager) Register(id string, ch channel.Channel) {
	m.channels[id] = ch
}

func (m *ChannelManager) Get(id string) channel.Channel {
	return m.channels[id]
}

func (m *ChannelManager) SetDefault(nodeID string, chType channel.ChannelType) {
	m.defaults[nodeID] = chType
}

func (m *ChannelManager) GetOrCreate(nodeID string, chType channel.ChannelType, opts ...interface{}) (channel.Channel, error) {
	if ch, ok := m.channels[nodeID]; ok {
		return ch, nil
	}

	var ch channel.Channel
	switch chType {
	case channel.ChannelMemory:
		ch = channel.NewMemoryChannel()
	case channel.ChannelFile:
		baseDir := "data"
		if len(opts) > 0 {
			baseDir = opts[0].(string)
		}
		var err error
		ch, err = channel.NewFileChannel(baseDir)
		if err != nil {
			return nil, err
		}
	case channel.ChannelNetwork:
		addr := "localhost:8080"
		if len(opts) > 0 {
			addr = opts[0].(string)
		}
		ch = channel.NewNetworkChannel(addr)
	default:
		ch = channel.NewMemoryChannel()
	}

	m.channels[nodeID] = ch
	return ch, nil
}

func (m *ChannelManager) CloseAll() error {
	for _, ch := range m.channels {
		ch.Close()
	}
	return nil
}
