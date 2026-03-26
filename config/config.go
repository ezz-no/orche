package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type WorkflowConfig struct {
	Name        string       `json:"name" yaml:"name"`
	Description string       `json:"description" yaml:"description"`
	Nodes       []NodeConfig `json:"nodes" yaml:"nodes"`
	Connections []Connection `json:"connections" yaml:"connections"`
	Config      ExecConfig   `json:"config" yaml:"config"`
}

type NodeConfig struct {
	ID       string            `json:"id" yaml:"id"`
	Type     string            `json:"type" yaml:"type"`
	Binary   string            `json:"binary" yaml:"binary"`
	Args     []string          `json:"args" yaml:"args"`
	Workdir  string            `json:"workdir" yaml:"workdir"`
	Env      map[string]string `json:"env" yaml:"env"`
	Endpoint string            `json:"endpoint" yaml:"endpoint"`
	Handler  string            `json:"handler" yaml:"handler"`
	Output   []PortConfig      `json:"output" yaml:"output"`
	Input    []InputConfig     `json:"input" yaml:"input"`
}

type PortConfig struct {
	Name  string `json:"name" yaml:"name"`
	Type  string `json:"type" yaml:"type"`
	Value string `json:"value" yaml:"value"`
}

type InputConfig struct {
	Name string `json:"name" yaml:"name"`
	From string `json:"from" yaml:"from"`
	Port string `json:"port" yaml:"port"`
}

type Connection struct {
	From     string `json:"from" yaml:"from"`
	To       string `json:"to" yaml:"to"`
	FromPort string `json:"from_port" yaml:"from_port"`
	ToPort   string `json:"to_port" yaml:"to_port"`
}

type ExecConfig struct {
	Parallelism int    `json:"parallelism" yaml:"parallelism"`
	RetryCount  int    `json:"retry_count" yaml:"retry_count"`
	RetryDelay  string `json:"retry_delay" yaml:"retry_delay"`
	ErrorMode   string `json:"error_mode" yaml:"error_mode"`
}

func Load(filename string) (*WorkflowConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var config WorkflowConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}
