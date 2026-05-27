package files

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Guard confines file access to a workspace root and blocks sensitive paths.
type Guard struct {
	root string
	deny []string
}

var defaultDeny = []string{".git", ".env", "secrets"}

// NewGuard creates a Guard rooted at root (resolved to an absolute, symlink-free path).
func NewGuard(root string) (*Guard, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("workspace root %q: %w", root, err)
	}
	return &Guard{root: resolved, deny: defaultDeny}, nil
}

// check rejects paths that resolve outside the root or match a denied segment.
func (g *Guard) check(p string) error {
	abs, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("cannot access %q: %w", p, err)
	}
	rel, err := filepath.Rel(g.root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("path %q is outside workspace root %q", p, g.root)
	}
	for _, seg := range strings.Split(rel, string(os.PathSeparator)) {
		for _, d := range g.deny {
			if seg == d {
				return fmt.Errorf("path %q is blocked (matches %q)", p, d)
			}
		}
	}
	return nil
}

// ReadAll expands the paths, rejects any that escape the root or match the deny-list,
// skips binary files (with a stderr warning), and reads the rest into Documents.
func (g *Guard) ReadAll(paths []string) ([]Document, error) {
	fileList, err := Expand(paths)
	if err != nil {
		return nil, err
	}
	var docs []Document
	for _, f := range fileList {
		if err := g.check(f); err != nil {
			return nil, err
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", f, err)
		}
		if isBinary(b) {
			fmt.Fprintf(os.Stderr, "claude-llama: skipping binary file %q\n", f)
			continue
		}
		docs = append(docs, Document{Path: f, Content: string(b)})
	}
	return docs, nil
}

// isBinary reports whether b looks binary (contains a NUL byte in the first 512 bytes).
func isBinary(b []byte) bool {
	n := len(b)
	if n > 512 {
		n = 512
	}
	return bytes.IndexByte(b[:n], 0) >= 0
}
