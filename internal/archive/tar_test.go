package archive

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"docksmith/internal/config"
)

func setupArchiveTest(t *testing.T) (tmpState string, tmpContext string) {
	t.Helper()
	tmpState, _ = os.MkdirTemp("", "docksmith-archive-state-*")
	tmpContext, _ = os.MkdirTemp("", "docksmith-archive-ctx-*")
	config.StateDir = tmpState
	os.MkdirAll(filepath.Join(tmpState, "layers"), 0755)
	return
}

// TestDeterministicTar verifies that creating a layer from the same files
// twice always produces an identical SHA-256 digest (byte-for-byte reproducibility).
func TestDeterministicTar(t *testing.T) {
	_, tmpCtx := setupArchiveTest(t)
	defer os.RemoveAll(tmpCtx)

	// Create test files
	os.WriteFile(filepath.Join(tmpCtx, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(tmpCtx, "b.txt"), []byte("world"), 0644)

	entries := map[string]string{
		filepath.Join(tmpCtx, "a.txt"): "a.txt",
		filepath.Join(tmpCtx, "b.txt"): "b.txt",
	}

	tmpState1, _ := os.MkdirTemp("", "ds1-*")
	tmpState2, _ := os.MkdirTemp("", "ds2-*")
	defer os.RemoveAll(tmpState1)
	defer os.RemoveAll(tmpState2)

	config.StateDir = tmpState1
	os.MkdirAll(filepath.Join(tmpState1, "layers"), 0755)
	r1, err := CreateLayer(entries)
	if err != nil {
		t.Fatalf("CreateLayer run 1 failed: %v", err)
	}

	config.StateDir = tmpState2
	os.MkdirAll(filepath.Join(tmpState2, "layers"), 0755)
	r2, err := CreateLayer(entries)
	if err != nil {
		t.Fatalf("CreateLayer run 2 failed: %v", err)
	}

	if r1.Digest != r2.Digest {
		t.Errorf("non-deterministic: run1=%s, run2=%s", r1.Digest, r2.Digest)
	}
}

// TestTarContentChangesDigest verifies different file contents produce different digests.
func TestTarContentChangesDigest(t *testing.T) {
	tmpState, tmpCtx := setupArchiveTest(t)
	defer os.RemoveAll(tmpState)
	defer os.RemoveAll(tmpCtx)

	os.WriteFile(filepath.Join(tmpCtx, "a.txt"), []byte("content-A"), 0644)
	entries1 := map[string]string{filepath.Join(tmpCtx, "a.txt"): "a.txt"}
	r1, _ := CreateLayer(entries1)

	os.WriteFile(filepath.Join(tmpCtx, "a.txt"), []byte("content-B"), 0644)
	r2, _ := CreateLayer(entries1)

	if r1.Digest == r2.Digest {
		t.Errorf("expected different digests for different content, got same: %s", r1.Digest)
	}
}

// TestExtractLayer verifies extraction round-trip: create a layer then extract it.
func TestExtractLayer(t *testing.T) {
	tmpState, tmpCtx := setupArchiveTest(t)
	defer os.RemoveAll(tmpState)
	defer os.RemoveAll(tmpCtx)

	content := []byte("important data")
	os.WriteFile(filepath.Join(tmpCtx, "hello.txt"), content, 0644)

	entries := map[string]string{filepath.Join(tmpCtx, "hello.txt"): "hello.txt"}
	result, err := CreateLayer(entries)
	if err != nil {
		t.Fatalf("CreateLayer failed: %v", err)
	}

	destDir, _ := os.MkdirTemp("", "docksmith-extract-*")
	defer os.RemoveAll(destDir)

	if err := ExtractLayer(result.Path, destDir); err != nil {
		t.Fatalf("ExtractLayer failed: %v", err)
	}

	extracted, err := os.ReadFile(filepath.Join(destDir, "hello.txt"))
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if !bytes.Equal(extracted, content) {
		t.Errorf("content mismatch: want %q, got %q", content, extracted)
	}
}

// TestTarSortOrder verifies entries are sorted regardless of map iteration
func TestTarSortOrder(t *testing.T) {
	// We'll verify that two layers built with same files (in different map order, irrelevant in maps)
	// produce identical tar byte streams
	_, tmpCtx := setupArchiveTest(t)
	defer os.RemoveAll(tmpCtx)

	os.MkdirAll(filepath.Join(tmpCtx, "z"), 0755)
	os.MkdirAll(filepath.Join(tmpCtx, "a"), 0755)
	os.WriteFile(filepath.Join(tmpCtx, "z", "file.txt"), []byte("zzz"), 0644)
	os.WriteFile(filepath.Join(tmpCtx, "a", "file.txt"), []byte("aaa"), 0644)

	readTar := func(stateDir string) []string {
		config.StateDir = stateDir
		os.MkdirAll(filepath.Join(stateDir, "layers"), 0755)
		entries := map[string]string{
			filepath.Join(tmpCtx, "z", "file.txt"): "z/file.txt",
			filepath.Join(tmpCtx, "a", "file.txt"): "a/file.txt",
		}
		r, _ := CreateLayer(entries)
		f, _ := os.Open(r.Path)
		defer f.Close()
		tr := tar.NewReader(f)
		var names []string
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			names = append(names, hdr.Name)
		}
		return names
	}

	s1, _ := os.MkdirTemp("", "sort1-*"); defer os.RemoveAll(s1)
	s2, _ := os.MkdirTemp("", "sort2-*"); defer os.RemoveAll(s2)

	n1 := readTar(s1)
	n2 := readTar(s2)

	if len(n1) != len(n2) {
		t.Fatalf("different entry counts: %d vs %d", len(n1), len(n2))
	}
	for i := range n1 {
		if n1[i] != n2[i] {
			t.Errorf("entry %d mismatch: %q vs %q", i, n1[i], n2[i])
		}
	}
	// Verify sorted order
	if len(n1) >= 2 && n1[0] > n1[1] {
		t.Errorf("entries not sorted: %v", n1)
	}
}
