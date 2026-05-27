package files

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

type Document struct {
	Path    string
	Content string
}

// Expand turns paths (files, globs, or directories) into a sorted, de-duplicated
// list of regular file paths. Directories are walked recursively. A literal path
// that does not exist returns an error.
func Expand(paths []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, p := range paths {
		matches, err := filepath.Glob(p)
		if err != nil {
			return nil, fmt.Errorf("bad glob %q: %w", p, err)
		}
		if matches == nil {
			matches = []string{p} // no glob match: treat as literal path
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil {
				return nil, fmt.Errorf("cannot access %q: %w", m, err)
			}
			if info.IsDir() {
				err := filepath.WalkDir(m, func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if !d.IsDir() {
						add(path)
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
			} else {
				add(m)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// ReadAll expands the given paths and reads each resulting file into a Document.
func ReadAll(paths []string) ([]Document, error) {
	fileList, err := Expand(paths)
	if err != nil {
		return nil, err
	}
	var docs []Document
	for _, f := range fileList {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", f, err)
		}
		docs = append(docs, Document{Path: f, Content: string(b)})
	}
	return docs, nil
}
