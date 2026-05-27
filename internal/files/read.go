package files

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
			if hasGlobMeta(p) {
				return nil, fmt.Errorf("glob %q matched no files", p)
			}
			matches = []string{p} // literal path: let os.Stat produce the error
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

// hasGlobMeta reports whether p contains any filepath glob metacharacters.
func hasGlobMeta(p string) bool {
	return strings.ContainsAny(p, "*?[")
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

// EstimateTokens approximates token count as len(s)/4 (rounded up).
func EstimateTokens(s string) int {
	return (len(s) + 3) / 4
}

// Chunk packs documents into chunks whose estimated token count stays under
// maxTokens. Each document is prefixed with a "// file: <path>" header.
// A single document larger than the budget is split across multiple chunks.
func Chunk(docs []Document, maxTokens int) []string {
	if maxTokens <= 0 {
		panic(fmt.Sprintf("Chunk: maxTokens must be positive, got %d", maxTokens))
	}
	maxChars := maxTokens * 4
	var chunks []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			chunks = append(chunks, b.String())
			b.Reset()
		}
	}
	for _, d := range docs {
		body := fmt.Sprintf("// file: %s\n%s\n", d.Path, d.Content)
		if len(body) <= maxChars {
			if b.Len()+len(body) > maxChars {
				flush()
			}
			b.WriteString(body)
			continue
		}
		flush()
		// Byte-slice the oversized document. This may split a multibyte UTF-8
		// rune at a boundary; acceptable for LLM consumption of mostly-ASCII
		// source files in v1.
		for start := 0; start < len(body); start += maxChars {
			end := start + maxChars
			if end > len(body) {
				end = len(body)
			}
			chunks = append(chunks, body[start:end])
		}
	}
	flush()
	return chunks
}
