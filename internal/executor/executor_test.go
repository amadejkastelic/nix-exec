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
				t.Errorf(
					"withInterpreterPackage(%q, %v) = %v, want %v",
					tt.lang,
					tt.pkgs,
					got,
					tt.want,
				)
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
	key1 := cacheKey("python", []string{"bash", "python3"})
	key2 := cacheKey("python", []string{"bash", "python3"})
	key3 := cacheKey("python", []string{"bash"})
	key4 := cacheKey("bash", []string{"bash", "python3"})

	if key1 != key2 {
		t.Error("cacheKey should be deterministic for same input")
	}

	if key1 == key3 {
		t.Error("cacheKey should differ for different package sets")
	}

	if key1 == key4 {
		t.Error("cacheKey should differ for different languages")
	}

	if len(key1) != 64 {
		t.Errorf("cacheKey should be 64 hex chars (sha256), got %d", len(key1))
	}
}

func TestGenerateFlake(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		flake := generateFlake(
			"bash",
			[]string{"bash", "ripgrep"},
			"github:NixOS/nixpkgs/nixpkgs-unstable",
		)
		if flake == "" {
			t.Fatal("generateFlake returned empty string")
		}
		wantPkgs := []string{"pkgs.bash", "pkgs.ripgrep"}
		for _, pkg := range wantPkgs {
			if !contains(flake, pkg) {
				t.Errorf("generated flake missing %q", pkg)
			}
		}
		if !contains(flake, "buildEnv") {
			t.Error("generated flake missing buildEnv")
		}
	})

	t.Run("python_with_packages", func(t *testing.T) {
		flake := generateFlake(
			"python",
			[]string{"python3", "python3Packages.pandas"},
			"github:NixOS/nixpkgs/nixpkgs-unstable",
		)
		if !contains(flake, "withPackages") {
			t.Error("python flake should use withPackages")
		}
		if !contains(flake, "ps.pandas") {
			t.Error("python flake should reference ps.pandas")
		}
		if contains(flake, "pkgs.python3Packages.pandas") {
			t.Error("python flake should not reference python3Packages directly in paths")
		}
	})

	t.Run("python_no_packages", func(t *testing.T) {
		flake := generateFlake(
			"python",
			[]string{"python3"},
			"github:NixOS/nixpkgs/nixpkgs-unstable",
		)
		if !contains(flake, "pkgs.python3\n") {
			t.Error("python flake without packages should use pkgs.python3 directly")
		}
		if contains(flake, "withPackages") {
			t.Error("python flake without packages should not use withPackages")
		}
	})
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
