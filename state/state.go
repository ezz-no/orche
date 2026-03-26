package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Execution struct {
	ID        string
	Graph     interface{}
	Status    string
	NodeCount int
	StartTime time.Time
	EndTime   time.Time
	Error     error
}

type Graph struct {
	Nodes    map[string]interface{} `json:"nodes"`
	Edges    []interface{}          `json:"edges"`
	Metadata map[string]string      `json:"metadata"`
}

type StateStore interface {
	SaveExecution(exec *Execution) error
	GetExecution(id string) (*Execution, error)
	SaveNodeState(nodeID string, outputs map[string][]byte) error
	GetNodeState(nodeID string) (map[string][]byte, error)
	SaveGraphState(graph *Graph) error
	GetGraphState() (*Graph, error)
	Clear() error
}

type FileStateStore struct {
	baseDir string
	mu      sync.RWMutex
}

func NewFileStateStore(baseDir string) (*FileStateStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}
	return &FileStateStore{baseDir: baseDir}, nil
}

func (s *FileStateStore) execPath(id string) string {
	return filepath.Join(s.baseDir, "executions", id+".json")
}

func (s *FileStateStore) nodeStatePath(nodeID string) string {
	return filepath.Join(s.baseDir, "nodes", nodeID+".json")
}

func (s *FileStateStore) SaveExecution(exec *Execution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(exec)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.execPath(exec.ID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(s.execPath(exec.ID), data, 0644)
}

func (s *FileStateStore) GetExecution(id string) (*Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.execPath(id))
	if err != nil {
		return nil, err
	}
	var exec Execution
	if err := json.Unmarshal(data, &exec); err != nil {
		return nil, err
	}
	return &exec, nil
}

func (s *FileStateStore) SaveNodeState(nodeID string, outputs map[string][]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(outputs)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.nodeStatePath(nodeID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(s.nodeStatePath(nodeID), data, 0644)
}

func (s *FileStateStore) GetNodeState(nodeID string) (map[string][]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.nodeStatePath(nodeID))
	if err != nil {
		return nil, err
	}
	var outputs map[string][]byte
	if err := json.Unmarshal(data, &outputs); err != nil {
		return nil, err
	}
	return outputs, nil
}

func (s *FileStateStore) SaveGraphState(g *Graph) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(g)
	if err != nil {
		return err
	}
	dir := filepath.Join(s.baseDir, "graph")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "graph.json"), data, 0644)
}

func (s *FileStateStore) GetGraphState() (*Graph, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(filepath.Join(s.baseDir, "graph", "graph.json"))
	if err != nil {
		return nil, err
	}
	var g Graph
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *FileStateStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.RemoveAll(s.baseDir)
}

type MemoryStateStore struct {
	mu         sync.RWMutex
	executions map[string]*Execution
	nodes      map[string]map[string][]byte
	graph      *Graph
}

func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{
		executions: make(map[string]*Execution),
		nodes:      make(map[string]map[string][]byte),
	}
}

func (s *MemoryStateStore) SaveExecution(exec *Execution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executions[exec.ID] = exec
	return nil
}

func (s *MemoryStateStore) GetExecution(id string) (*Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if exec, ok := s.executions[id]; ok {
		return exec, nil
	}
	return nil, fmt.Errorf("execution not found: %s", id)
}

func (s *MemoryStateStore) SaveNodeState(nodeID string, outputs map[string][]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[nodeID] = outputs
	return nil
}

func (s *MemoryStateStore) GetNodeState(nodeID string) (map[string][]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if outputs, ok := s.nodes[nodeID]; ok {
		return outputs, nil
	}
	return nil, fmt.Errorf("node state not found: %s", nodeID)
}

func (s *MemoryStateStore) SaveGraphState(g *Graph) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.graph = g
	return nil
}

func (s *MemoryStateStore) GetGraphState() (*Graph, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.graph, nil
}

func (s *MemoryStateStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executions = make(map[string]*Execution)
	s.nodes = make(map[string]map[string][]byte)
	return nil
}
