package channel

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ChannelType int

const (
	ChannelMemory ChannelType = iota
	ChannelFile
	ChannelNetwork
)

type Channel interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	Close() error
	Type() ChannelType
}

type MemoryChannel struct {
	mu   sync.RWMutex
	data map[string][]byte
	cond sync.Cond
}

func NewMemoryChannel() *MemoryChannel {
	ch := &MemoryChannel{
		data: make(map[string][]byte),
	}
	ch.cond.L = &ch.mu
	return ch
}

func (c *MemoryChannel) Type() ChannelType { return ChannelMemory }

func (c *MemoryChannel) Put(ctx context.Context, key string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = data
	c.cond.Broadcast()
	return nil
}

func (c *MemoryChannel) Get(ctx context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	data, ok := c.data[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return data, nil
}

func (c *MemoryChannel) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
	return nil
}

func (c *MemoryChannel) Close() error {
	return nil
}

type FileChannel struct {
	baseDir string
	mu      sync.RWMutex
}

func NewFileChannel(baseDir string) (*FileChannel, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}
	return &FileChannel{baseDir: baseDir}, nil
}

func (c *FileChannel) Type() ChannelType { return ChannelFile }

func (c *FileChannel) path(key string) string {
	return filepath.Join(c.baseDir, key)
}

func (c *FileChannel) Put(ctx context.Context, key string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	path := c.path(key)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c *FileChannel) Get(ctx context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return os.ReadFile(c.path(key))
}

func (c *FileChannel) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return os.Remove(c.path(key))
}

func (c *FileChannel) Close() error {
	return os.RemoveAll(c.baseDir)
}

type NetworkChannel struct {
	addr      string
	listener  net.Listener
	mu        sync.RWMutex
	pending   chan []byte
	connected bool
	wg        sync.WaitGroup
}

func NewNetworkChannel(addr string) *NetworkChannel {
	return &NetworkChannel{
		addr:    addr,
		pending: make(chan []byte, 100),
	}
}

func (c *NetworkChannel) Type() ChannelType { return ChannelNetwork }

func (c *NetworkChannel) Start() error {
	ln, err := net.Listen("tcp", c.addr)
	if err != nil {
		return err
	}
	c.listener = ln
	c.connected = true

	c.wg.Add(1)
	go c.acceptLoop()
	return nil
}

func (c *NetworkChannel) acceptLoop() {
	defer c.wg.Done()
	for {
		conn, err := c.listener.Accept()
		if err != nil {
			return
		}
		go c.handleConn(conn)
	}
}

func (c *NetworkChannel) handleConn(conn net.Conn) {
	defer conn.Close()
	data, err := io.ReadAll(conn)
	if err != nil {
		return
	}
	c.pending <- data
}

func (c *NetworkChannel) Put(ctx context.Context, key string, data []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.connected {
		return fmt.Errorf("channel not connected")
	}
	conn, err := net.Dial("tcp", c.addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write(data)
	return err
}

func (c *NetworkChannel) Get(ctx context.Context, key string) ([]byte, error) {
	select {
	case data := <-c.pending:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout waiting for data")
	}
}

func (c *NetworkChannel) Delete(ctx context.Context, key string) error {
	return nil
}

func (c *NetworkChannel) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
	if c.listener != nil {
		c.listener.Close()
	}
	c.wg.Wait()
	return nil
}
