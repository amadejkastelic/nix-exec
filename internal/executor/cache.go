package executor

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

type EnvCache struct {
	mu     sync.RWMutex
	items  map[string]string
	dir    string
	logger *slog.Logger
}

func NewEnvCache(dir string, logger *slog.Logger) *EnvCache {
	c := &EnvCache{
		items:  make(map[string]string),
		dir:    dir,
		logger: logger,
	}
	c.load()
	return c
}

func (c *EnvCache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	p, ok := c.items[key]
	if !ok {
		return "", false
	}

	if _, err := os.Stat(p); err != nil {
		c.logger.Debug("cached path no longer exists, evicting", "key", key, "path", p)
		delete(c.items, key)
		return "", false
	}

	return p, true
}

func (c *EnvCache) Set(key, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = path
	c.persist()
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
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		c.logger.Warn("failed to create cache directory", "error", err)
		return
	}

	data, err := json.MarshalIndent(c.items, "", "  ")
	if err != nil {
		c.logger.Warn("failed to marshal cache", "error", err)
		return
	}

	if err := os.WriteFile(c.filePath(), data, 0644); err != nil {
		c.logger.Warn("failed to write cache file", "error", err)
	}
}

func (c *EnvCache) filePath() string {
	return filepath.Join(c.dir, "env-cache.json")
}
