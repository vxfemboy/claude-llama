package files

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExpandFileGlobAndDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "A")
	writeFile(t, filepath.Join(dir, "b.txt"), "B")
	writeFile(t, filepath.Join(dir, "sub", "c.txt"), "C")

	// directory expands recursively
	got, err := Expand([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("dir expand got %d files, want 3: %v", len(got), got)
	}

	// glob expands
	got, err = Expand([]string{filepath.Join(dir, "*.txt")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("glob expand got %d files, want 2: %v", len(got), got)
	}
}

func TestExpandMissingPathErrors(t *testing.T) {
	_, err := Expand([]string{filepath.Join(t.TempDir(), "nope.txt")})
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}

func TestExpandDeduplicates(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "A")
	p := filepath.Join(dir, "a.txt")
	got, err := Expand([]string{p, p})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("Expand with duplicate path got %d files, want 1: %v", len(got), got)
	}
}

func TestExpandGlobNoMatchErrors(t *testing.T) {
	dir := t.TempDir()
	_, err := Expand([]string{filepath.Join(dir, "*.go")})
	if err == nil {
		t.Fatal("expected error for glob matching no files, got nil")
	}
}

func TestEstimateTokens(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"", 0},
		{"1", 1},
		{"12345", 2},
		{"12345678", 2},
	} {
		if got := EstimateTokens(tc.in); got != tc.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestChunkPacksSmallDocs(t *testing.T) {
	docs := []Document{
		{Path: "a", Content: "aaaa"},
		{Path: "b", Content: "bbbb"},
	}
	chunks := Chunk(docs, 1000)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if !strings.Contains(chunks[0], "// file: a") || !strings.Contains(chunks[0], "// file: b") {
		t.Errorf("chunk missing file headers: %q", chunks[0])
	}
	if !strings.Contains(chunks[0], "aaaa") || !strings.Contains(chunks[0], "bbbb") {
		t.Errorf("chunk missing document content: %q", chunks[0])
	}
}

func TestChunkSplitsOversizedDoc(t *testing.T) {
	docs := []Document{{Path: "big", Content: strings.Repeat("x", 400)}}
	chunks := Chunk(docs, 25) // 100 chars/chunk => multiple chunks
	if len(chunks) < 2 {
		t.Fatalf("got %d chunks, want >= 2", len(chunks))
	}
}

func TestChunkEmptyDocs(t *testing.T) {
	if chunks := Chunk(nil, 1000); chunks != nil {
		t.Errorf("Chunk(nil) = %v, want nil", chunks)
	}
}
