package executor

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

type EnvCache struct {
	mu      sync.RWMutex
	items   map[string]string
	order   []string
	dir     string
	maxSize int
	logger  *slog.Logger
}

func NewEnvCache(dir string, maxSize int, logger *slog.Logger) *EnvCache {
	c := &EnvCache{
		items:   make(map[string]string),
		order:   make([]string, 0),
		dir:     dir,
		maxSize: maxSize,
		logger:  logger,
	}
	c.load()
	return c
}

func (c *EnvCache) Get(key string) (string, bool) {
	c.mu.RLock()
	p, ok := c.items[key]
	c.mu.RUnlock()

	if !ok {
		return "", false
	}

	if _, err := os.Stat(p); err != nil {
		c.evict(key, p)
		return "", false
	}

	return p, true
}

func (c *EnvCache) evict(key, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cur, ok := c.items[key]; ok && cur == path {
		delete(c.items, key)
		c.removeFromOrder(key)
		c.persist()
	}
}

func (c *EnvCache) Set(key, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.items[key]; !exists {
		c.order = append(c.order, key)
	}

	c.items[key] = path
	c.enforceMax()
	c.persist()
}

func (c *EnvCache) enforceMax() {
	if c.maxSize <= 0 {
		return
	}
	for len(c.items) > c.maxSize && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.items, oldest)
	}
}

func (c *EnvCache) removeFromOrder(key string) {
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}

func (c *EnvCache) load() {
	p := c.filePath()
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}

	if err := json.Unmarshal(data, &c.items); err != nil {
		c.logger.Warn("failed to parse cache file, starting fresh", "error", err)
		c.items = make(map[string]string)
	}
}

func (c *EnvCache) persist() {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		c.logger.Warn("failed to create cache directory", "error", err)
		return
	}

	data, err := json.MarshalIndent(c.items, "", "  ")
	if err != nil {
		c.logger.Warn("failed to marshal cache", "error", err)
		return
	}

	if err := os.WriteFile(c.filePath(), data, 0o644); err != nil {
		c.logger.Warn("failed to write cache file", "error", err)
	}
}

func (c *EnvCache) filePath() string {
	return filepath.Join(c.dir, "env-cache.json")
}
