package executor

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestCacheSetAndGet(t *testing.T) {
	dir := t.TempDir()

	realPath := filepath.Join(dir, "env")
	if err := os.MkdirAll(realPath, 0o755); err != nil {
		t.Fatal(err)
	}

	c := NewEnvCache(dir, 0, slog.Default())
	c.Set("key1", realPath)

	if p, ok := c.Get("key1"); !ok || p != realPath {
		t.Errorf("expected %s, got %q, ok=%v", realPath, p, ok)
	}
}

func TestCacheEvictsStale(t *testing.T) {
	dir := t.TempDir()
	c := NewEnvCache(dir, 0, slog.Default())

	c.Set("key1", "/nix/store/nonexistent")
	if _, ok := c.Get("key1"); ok {
		t.Error("expected stale entry to be evicted")
	}
}

func TestCacheMaxSize(t *testing.T) {
	dir := t.TempDir()
	c := NewEnvCache(dir, 3, slog.Default())

	c.Set("a", "/nix/store/a")
	c.Set("b", "/nix/store/b")
	c.Set("c", "/nix/store/c")

	if len(c.items) != 3 {
		t.Errorf("expected 3 items, got %d", len(c.items))
	}

	c.Set("d", "/nix/store/d")

	if len(c.items) > 3 {
		t.Errorf("expected at most 3 items, got %d", len(c.items))
	}

	if _, ok := c.items["a"]; ok {
		t.Error("expected oldest entry 'a' to be evicted")
	}
}

func TestCacheMaxSizeZeroUnlimited(t *testing.T) {
	dir := t.TempDir()
	c := NewEnvCache(dir, 0, slog.Default())

	for i := 0; i < 100; i++ {
		key := string(rune('a'+i%26)) + string(rune('0'+i/26))
		c.Set(key, "/nix/store/"+key)
	}

	if len(c.items) != 100 {
		t.Errorf("expected 100 items with no limit, got %d", len(c.items))
	}
}

func TestCacheOverwriteKeepsSize(t *testing.T) {
	dir := t.TempDir()
	c := NewEnvCache(dir, 2, slog.Default())

	c.Set("a", "/nix/store/a")
	c.Set("b", "/nix/store/b")
	c.Set("b", "/nix/store/b-v2")

	if len(c.items) != 2 {
		t.Errorf("expected 2 items after overwrite, got %d", len(c.items))
	}
	if c.items["b"] != "/nix/store/b-v2" {
		t.Errorf("expected updated value, got %s", c.items["b"])
	}
}

func TestCachePersists(t *testing.T) {
	dir := t.TempDir()

	realPath := filepath.Join(dir, "env")
	if err := os.MkdirAll(realPath, 0o755); err != nil {
		t.Fatal(err)
	}

	c1 := NewEnvCache(dir, 0, slog.Default())
	c1.Set("key1", realPath)

	c2 := NewEnvCache(dir, 0, slog.Default())
	if p, ok := c2.Get("key1"); !ok || p != realPath {
		t.Errorf("expected persisted value, got %q, ok=%v", p, ok)
	}
}

func TestCacheEvictRemovesFromOrder(t *testing.T) {
	dir := t.TempDir()
	c := NewEnvCache(dir, 0, slog.Default())

	c.Set("a", "/nix/store/nonexistent-a")
	c.Set("b", "/nix/store/nonexistent-b")

	c.Get("a")

	if _, ok := c.items["a"]; ok {
		t.Error("expected 'a' to be evicted after stat failure")
	}
	if len(c.order) != 1 || c.order[0] != "b" {
		t.Errorf("expected order [b], got %v", c.order)
	}
}

func TestCacheExpandsHomeDir(t *testing.T) {
	dir := t.TempDir()

	c := NewEnvCache(dir, 0, slog.Default())
	c.Set("key1", "/nix/store/test")

	data, err := os.ReadFile(filepath.Join(dir, "env-cache.json"))
	if err != nil {
		t.Fatalf("cache file not created: %v", err)
	}
	if len(data) == 0 {
		t.Error("cache file is empty")
	}
}
