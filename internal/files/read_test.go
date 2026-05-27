package files

import (
	"os"
	"path/filepath"
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

func TestReadAll(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "hello")

	docs, err := ReadAll([]string{filepath.Join(dir, "a.txt")})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Content != "hello" {
		t.Fatalf("ReadAll = %+v", docs)
	}
}
