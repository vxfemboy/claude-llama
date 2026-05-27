package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGuardReadAllWithinRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "hello")

	g, err := NewGuard(dir)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := g.ReadAll([]string{filepath.Join(dir, "a.txt")})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Content != "hello" {
		t.Fatalf("ReadAll = %+v", docs)
	}
}

func TestGuardRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.txt"), "x")

	g, err := NewGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.ReadAll([]string{filepath.Join(outside, "secret.txt")})
	if err == nil {
		t.Fatal("expected error reading outside workspace root")
	}
}

func TestGuardRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "target.txt"), "secret")
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(filepath.Join(outside, "target.txt"), link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	g, err := NewGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.ReadAll([]string{link})
	if err == nil {
		t.Fatal("expected error for symlink escaping workspace root")
	}
}

func TestGuardDeniesSensitivePaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".env"), "SECRET=1")

	g, err := NewGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.ReadAll([]string{filepath.Join(root, ".env")})
	if err == nil {
		t.Fatal("expected .env to be denied")
	}
}

func TestGuardDeniesSensitivePathsNested(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "project", ".git", "config"), "x")

	g, err := NewGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.ReadAll([]string{filepath.Join(root, "project", ".git", "config")})
	if err == nil {
		t.Fatal("expected nested .git/config to be denied")
	}
}

func TestIsBinaryBoundary(t *testing.T) {
	// NUL within the first 512 bytes => binary.
	within := make([]byte, 600)
	within[100] = 0x00
	if !isBinary(within) {
		t.Error("expected NUL at byte 100 to be detected as binary")
	}
	// NUL only after the first 512 bytes => not detected (documented heuristic limit).
	after := make([]byte, 600)
	after[513] = 0x00
	if isBinary(after) {
		t.Error("expected NUL at byte 513 to be outside the 512-byte scan window")
	}
}

func TestGuardSkipsBinaryFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "text.txt"), "readable")
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte{0x00, 0x01, 0x02, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}

	g, err := NewGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := g.ReadAll([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Content != "readable" {
		t.Fatalf("expected only the text file, got %+v", docs)
	}
}
