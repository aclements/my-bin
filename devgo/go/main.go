// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command go is a wrapper for the Go toolchain that automatically delegates
// to a development Go tree when working inside one.
//
// Specifically, if the current working directory is within a Go development
// tree authorized by the GODEVROOTS environment variable, this command
// execs bin/go from that development tree. Otherwise, it delegates to the next
// go executable found on PATH.
//
// GODEVROOTS is a list of absolute paths or glob patterns separated by the
// system list separator (':'). A directory is recognized as an authorized Go
// development tree root if:
//  1. It matches an exact path or glob pattern in GODEVROOTS (where '*' matches
//     within a path component and does not cross '/').
//  2. It contains src/go.mod declaring "module std".
//  3. It contains an executable bin/go.
//
// When delegating to the next go executable on PATH, this wrapper avoids
// recursive loops by executing the first candidate appearing strictly after
// itself in PATH order.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {
	if target := findDevGo(); target != "" {
		execGo(target)
	}

	nextGo, err := findNextGo()
	if err != nil {
		cwd, _ := os.Getwd()
		fmt.Fprintln(os.Stderr, explainNoGo(cwd, os.Getenv("GODEVROOTS")))
		os.Exit(127)
	}
	execGo(nextGo)
}

// findDevGo returns the path to bin/go in an authorized Go development tree
// enclosing the current working directory, or an empty string if none is found.
func findDevGo() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return findDevGoFrom(os.Getenv("GODEVROOTS"), cwd)
}

// findDevGoFrom searches upward from cwd for a Go development tree whose root
// matches one of the patterns in roots.
func findDevGoFrom(roots, cwd string) string {
	if roots == "" || cwd == "" {
		return ""
	}

	treeRoot := findDevRoot(cwd)
	if treeRoot == "" {
		if realCwd, err := filepath.EvalSymlinks(cwd); err == nil && realCwd != cwd {
			treeRoot = findDevRoot(realCwd)
		}
	}
	if treeRoot == "" {
		return ""
	}

	realRoot, _ := filepath.EvalSymlinks(treeRoot)
	for pat := range strings.SplitSeq(roots, string(filepath.ListSeparator)) {
		if pat == "" {
			continue
		}
		// filepath.Match handles exact paths and globs (* never matches /)
		if match(pat, treeRoot) || (realRoot != "" && match(pat, realRoot)) {
			return filepath.Join(treeRoot, "bin", "go")
		}
	}
	return ""
}

// match reports whether path matches pattern using path-aware glob syntax.
func match(pat, path string) bool {
	matched, _ := filepath.Match(pat, path)
	return matched
}

// findDevRoot walks upward from dir to find the root of a ready-to-run Go
// development tree (containing both src/go.mod with module std and an
// executable bin/go).
func findDevRoot(dir string) string {
	for {
		if isDevTree(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// isDevTree reports whether dir is the root of a built Go development tree.
func isDevTree(dir string) bool {
	bin := filepath.Join(dir, "bin", "go")
	if fi, err := os.Stat(bin); err != nil || fi.IsDir() || fi.Mode()&0111 == 0 {
		return false
	}
	return hasStdMod(dir)
}

// findDevTreeRoot walks upward from dir to find the root of a Go development
// tree, regardless of whether bin/go has been built yet.
func findDevTreeRoot(dir string) string {
	for {
		if hasStdMod(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// hasStdMod reports whether dir/src/go.mod exists and declares "module std".
func hasStdMod(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "src", "go.mod"))
	if err != nil {
		return false
	}
	for line := range strings.Lines(string(data)) {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "module" && f[1] == "std" {
			return true
		}
	}
	return false
}

// explainNoGo returns a descriptive error message explaining why no go binary
// could be executed for cwd and the given GODEVROOTS setting.
func explainNoGo(cwd, godevroots string) string {
	treeRoot := findDevTreeRoot(cwd)
	if treeRoot == "" {
		if realCwd, err := filepath.EvalSymlinks(cwd); err == nil && realCwd != cwd {
			treeRoot = findDevTreeRoot(realCwd)
		}
	}

	if treeRoot != "" {
		binGo := filepath.Join(treeRoot, "bin", "go")
		if fi, err := os.Stat(binGo); err != nil || fi.IsDir() || fi.Mode()&0111 == 0 {
			return fmt.Sprintf("go: %s/bin/go not found or not executable (tree not built?) and no other \"go\" binary found in PATH", treeRoot)
		}
		if godevroots == "" {
			return fmt.Sprintf("go: %s is a Go development tree, but GODEVROOTS is unset and no other \"go\" binary found in PATH", treeRoot)
		}
		return fmt.Sprintf("go: %s is not in GODEVROOTS and no other \"go\" binary found in PATH", treeRoot)
	}

	return "go: not in a Go development tree and no other \"go\" binary found in PATH"
}

// findNextGo returns the path to the next go executable on PATH after the
// current executable.
func findNextGo() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	return findNextGoFrom(os.Getenv("PATH"), self)
}

// findNextGoFrom iterates over pathEnv to locate the first executable go
// appearing strictly after self (comparing file identity via os.SameFile).
// If self is not in pathEnv (e.g. invoked directly by path), it returns
// the first executable go found.
func findNextGoFrom(pathEnv, self string) (string, error) {
	selfFi, _ := os.Stat(self)

	foundSelf := false
	var fallback string

	for dir := range strings.SplitSeq(pathEnv, string(filepath.ListSeparator)) {
		if dir == "" {
			dir = "."
		}
		cand := filepath.Join(dir, "go")
		fi, err := os.Stat(cand)
		if err != nil || fi.IsDir() || fi.Mode()&0111 == 0 {
			continue
		}

		if selfFi != nil && os.SameFile(selfFi, fi) {
			foundSelf = true
			continue
		}
		if foundSelf {
			return cand, nil
		}
		if fallback == "" {
			fallback = cand
		}
	}

	if !foundSelf && fallback != "" {
		return fallback, nil
	}
	return "", errors.New("go: command not found")
}

// execGo replaces the current process with binary using syscall.Exec.
func execGo(binary string) {
	if err := syscall.Exec(binary, os.Args, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "exec %s: %v\n", binary, err)
		os.Exit(1)
	}
}
