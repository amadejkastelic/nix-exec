package executor

import (
	"testing"
)

func TestResolveInterpreter(t *testing.T) {
	tests := []struct {
		lang string
		want string
		err  bool
	}{
		{"python", "python3", false},
		{"bash", "bash", false},
		{"node", "node", false},
		{"rust", "", true},
	}

	for _, tt := range tests {
		got, err := resolveInterpreter(tt.lang)
		if tt.err && err == nil {
			t.Errorf("resolveInterpreter(%q) expected error, got nil", tt.lang)
		}
		if !tt.err && got != tt.want {
			t.Errorf("resolveInterpreter(%q) = %q, want %q", tt.lang, got, tt.want)
		}
	}
}

func TestWithInterpreterPackage(t *testing.T) {
	tests := []struct {
		lang string
		pkgs []string
		want []string
	}{
		{"python", nil, []string{"python3"}},
		{"python", []string{"python3"}, []string{"python3"}},
		{"bash", []string{"curl"}, []string{"bash", "curl"}},
		{"node", []string{"ripgrep"}, []string{"nodejs", "ripgrep"}},
	}

	for _, tt := range tests {
		got := withInterpreterPackage(tt.lang, tt.pkgs)
		if len(got) != len(tt.want) {
			t.Errorf("withInterpreterPackage(%q, %v) = %v, want %v", tt.lang, tt.pkgs, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("withInterpreterPackage(%q, %v) = %v, want %v", tt.lang, tt.pkgs, got, tt.want)
				break
			}
		}
	}
}

func TestScriptExtension(t *testing.T) {
	tests := []struct {
		lang string
		want string
	}{
		{"python", ".py"},
		{"bash", ".sh"},
		{"node", ".js"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		got := scriptExtension(tt.lang)
		if got != tt.want {
			t.Errorf("scriptExtension(%q) = %q, want %q", tt.lang, got, tt.want)
		}
	}
}

func TestCacheKey(t *testing.T) {
	key1 := cacheKey([]string{"bash", "python3"})
	key2 := cacheKey([]string{"bash", "python3"})
	key3 := cacheKey([]string{"bash"})

	if key1 != key2 {
		t.Error("cacheKey should be deterministic for same input")
	}

	if key1 == key3 {
		t.Error("cacheKey should differ for different package sets")
	}

	if len(key1) != 64 {
		t.Errorf("cacheKey should be 64 hex chars (sha256), got %d", len(key1))
	}
}

func TestGenerateFlake(t *testing.T) {
	flake := generateFlake([]string{"python3", "ripgrep"}, "github:NixOS/nixpkgs/nixpkgs-unstable")

	if flake == "" {
		t.Fatal("generateFlake returned empty string")
	}

	wantPkgs := []string{"pkgs.python3", "pkgs.ripgrep"}
	for _, pkg := range wantPkgs {
		if !contains(flake, pkg) {
			t.Errorf("generated flake missing %q", pkg)
		}
	}

	if !contains(flake, "buildEnv") {
		t.Error("generated flake missing buildEnv")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
