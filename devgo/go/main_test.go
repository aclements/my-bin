package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeSkeletonTree creates a fake Go development tree under dir.
func makeSkeletonTree(t *testing.T, dir string, modContent string, executable bool) string {
	t.Helper()
	srcDir := filepath.Join(dir, "src")
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}

	if modContent != "" {
		if err := os.WriteFile(filepath.Join(srcDir, "go.mod"), []byte(modContent), 0644); err != nil {
			t.Fatal(err)
		}
	}

	binGo := filepath.Join(binDir, "go")
	perm := os.FileMode(0644)
	if executable {
		perm = 0755
	}
	if err := os.WriteFile(binGo, []byte("#!/bin/sh\nexit 0\n"), perm); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestMatch(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/home/austin/go.dev", "/home/austin/go.dev", true},
		{"/home/austin/go.dev", "/home/austin/go.dev2", false},
		{"/home/austin/trees/*", "/home/austin/trees/dev1", true},
		{"/home/austin/trees/*", "/home/austin/trees/dev2", true},
		// Star does not match across /
		{"/home/austin/trees/*", "/home/austin/trees/sub/dev1", false},
		{"/home/austin/go-*", "/home/austin/go-1.26", true},
		{"/home/austin/go-*", "/home/austin/other", false},
	}

	for _, tt := range tests {
		got := match(tt.pattern, tt.path)
		if got != tt.want {
			t.Errorf("match(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

func TestIsDevTree(t *testing.T) {
	t.Run("valid tree", func(t *testing.T) {
		tmp := t.TempDir()
		makeSkeletonTree(t, tmp, "module std\n\ngo 1.26\n", true)
		if !isDevTree(tmp) {
			t.Errorf("isDevTree(%q) = false, want true", tmp)
		}
	})

	t.Run("non-executable bin/go", func(t *testing.T) {
		tmp := t.TempDir()
		makeSkeletonTree(t, tmp, "module std\n\ngo 1.26\n", false)
		if isDevTree(tmp) {
			t.Errorf("isDevTree(%q) = true, want false (not executable)", tmp)
		}
	})

	t.Run("missing src/go.mod", func(t *testing.T) {
		tmp := t.TempDir()
		makeSkeletonTree(t, tmp, "", true)
		if isDevTree(tmp) {
			t.Errorf("isDevTree(%q) = true, want false (missing go.mod)", tmp)
		}
	})

	t.Run("different module name", func(t *testing.T) {
		tmp := t.TempDir()
		makeSkeletonTree(t, tmp, "module example.com/foo\n", true)
		if isDevTree(tmp) {
			t.Errorf("isDevTree(%q) = true, want false (module is not std)", tmp)
		}
	})

	t.Run("module std with comments and spaces", func(t *testing.T) {
		tmp := t.TempDir()
		makeSkeletonTree(t, tmp, "// comment\n   module   std   \ngo 1.26\n", true)
		if !isDevTree(tmp) {
			t.Errorf("isDevTree(%q) = false, want true", tmp)
		}
	})
}

func TestFindDevRoot(t *testing.T) {
	tmp := t.TempDir()
	tree := filepath.Join(tmp, "go.dev")
	makeSkeletonTree(t, tree, "module std\n", true)

	subPkg := filepath.Join(tree, "src", "net", "http")
	if err := os.MkdirAll(subPkg, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		dir  string
		want string
	}{
		{"from root", tree, tree},
		{"from src", filepath.Join(tree, "src"), tree},
		{"from deep subpkg", subPkg, tree},
		{"from outside", tmp, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findDevRoot(tt.dir)
			if got != tt.want {
				t.Errorf("findDevRoot(%q) = %q, want %q", tt.dir, got, tt.want)
			}
		})
	}
}

func TestFindDevGoFrom(t *testing.T) {
	tmp := t.TempDir()
	tree1 := filepath.Join(tmp, "tree1")
	tree2 := filepath.Join(tmp, "tree2")
	makeSkeletonTree(t, tree1, "module std\n", true)
	makeSkeletonTree(t, tree2, "module std\n", true)

	sub1 := filepath.Join(tree1, "src", "cmd", "compile")
	if err := os.MkdirAll(sub1, 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("matching exact root", func(t *testing.T) {
		roots := tree1
		got := findDevGoFrom(roots, sub1)
		want := filepath.Join(tree1, "bin", "go")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("matching glob root", func(t *testing.T) {
		roots := filepath.Join(tmp, "*")
		got := findDevGoFrom(roots, sub1)
		want := filepath.Join(tree1, "bin", "go")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("not in GODEVROOTS", func(t *testing.T) {
		roots := tree2
		got := findDevGoFrom(roots, sub1)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("empty GODEVROOTS", func(t *testing.T) {
		got := findDevGoFrom("", sub1)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestFindNextGoFrom(t *testing.T) {
	tmp := t.TempDir()
	d1 := filepath.Join(tmp, "d1")
	d2 := filepath.Join(tmp, "d2")
	d3 := filepath.Join(tmp, "d3")
	for _, d := range []string{d1, d2, d3} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "go"), []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	pathEnv := strings.Join([]string{d1, d2, d3}, string(filepath.ListSeparator))

	t.Run("self is first", func(t *testing.T) {
		self := filepath.Join(d1, "go")
		got, err := findNextGoFrom(pathEnv, self)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(d2, "go")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("self is middle (avoids loop to d1)", func(t *testing.T) {
		self := filepath.Join(d2, "go")
		got, err := findNextGoFrom(pathEnv, self)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(d3, "go")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("self is last", func(t *testing.T) {
		self := filepath.Join(d3, "go")
		_, err := findNextGoFrom(pathEnv, self)
		if err == nil {
			t.Errorf("expected error, got nil")
		}
	})

	t.Run("self not in PATH", func(t *testing.T) {
		dOther := filepath.Join(tmp, "other")
		if err := os.MkdirAll(dOther, 0755); err != nil {
			t.Fatal(err)
		}
		self := filepath.Join(dOther, "go")
		if err := os.WriteFile(self, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}

		got, err := findNextGoFrom(pathEnv, self)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(d1, "go")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("os.SameFile via symlink to self", func(t *testing.T) {
		// Create d0 with a symlink to d2/go.
		d0 := filepath.Join(tmp, "d0")
		if err := os.MkdirAll(d0, 0755); err != nil {
			t.Fatal(err)
		}
		symlinkGo := filepath.Join(d0, "go")
		if err := os.Symlink(filepath.Join(d2, "go"), symlinkGo); err != nil {
			t.Fatal(err)
		}

		// PATH has d1, d0 (symlink to d2), d3.
		customPath := strings.Join([]string{d1, d0, d3}, string(filepath.ListSeparator))
		self := filepath.Join(d2, "go") // self points to the target of the symlink in d0

		// findNextGoFrom should recognize d0/go is the same file as self, and return d3/go!
		got, err := findNextGoFrom(customPath, self)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(d3, "go")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("os.SameFile via hard link to self", func(t *testing.T) {
		// Create d0 with a hard link to d2/go.
		d0 := filepath.Join(tmp, "d0_hard")
		if err := os.MkdirAll(d0, 0755); err != nil {
			t.Fatal(err)
		}
		hardlinkGo := filepath.Join(d0, "go")
		if err := os.Link(filepath.Join(d2, "go"), hardlinkGo); err != nil {
			t.Fatal(err)
		}

		customPath := strings.Join([]string{d1, d0, d3}, string(filepath.ListSeparator))
		self := filepath.Join(d2, "go")

		got, err := findNextGoFrom(customPath, self)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(d3, "go")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestExplainNoGo(t *testing.T) {
	tmp := t.TempDir()
	tree := filepath.Join(tmp, "go.dev")
	makeSkeletonTree(t, tree, "module std\n", true)

	t.Run("outside dev tree", func(t *testing.T) {
		got := explainNoGo(tmp, "/some/root")
		want := "go: not in a Go development tree and no other \"go\" binary found in PATH"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("inside dev tree, GODEVROOTS unset", func(t *testing.T) {
		got := explainNoGo(tree, "")
		want := fmt.Sprintf("go: %s is a Go development tree, but GODEVROOTS is unset and no other \"go\" binary found in PATH", tree)
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("inside dev tree, not in GODEVROOTS", func(t *testing.T) {
		got := explainNoGo(tree, "/other/path")
		want := fmt.Sprintf("go: %s is not in GODEVROOTS and no other \"go\" binary found in PATH", tree)
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("inside dev tree, bin/go not executable", func(t *testing.T) {
		unbuilt := filepath.Join(tmp, "unbuilt")
		makeSkeletonTree(t, unbuilt, "module std\n", false)
		got := explainNoGo(unbuilt, unbuilt)
		want := fmt.Sprintf("go: %s/bin/go not found or not executable (tree not built?) and no other \"go\" binary found in PATH", unbuilt)
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
