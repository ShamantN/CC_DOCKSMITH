package archive

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateLayerReproducibility(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "docksmith-repro-*")
	defer os.RemoveAll(tmpDir)

	// Create a set of files with weird permissions and 0-bytes
	file1 := filepath.Join(tmpDir, "empty.txt")
	os.WriteFile(file1, []byte(""), 0400) // Read-only, empty

	dir1 := filepath.Join(tmpDir, "nested/empty/dir")
	os.MkdirAll(dir1, 0700)

	file2 := filepath.Join(tmpDir, "exec.sh")
	os.WriteFile(file2, []byte("#!/bin/sh\necho hi"), 0777)

	entries := map[string]string{
		file1: "empty.txt",
		dir1:  "nested/empty/dir",
		file2: "exec.sh",
	}

	res1, err := CreateLayer(entries)
	if err != nil {
		t.Fatalf("Failed to create layer 1: %v", err)
	}

	// Create the EXACT SAME logical layout in a DIFFERENT physical directory
	tmpDir2, _ := os.MkdirTemp("", "docksmith-repro-2-*")
	defer os.RemoveAll(tmpDir2)

	file1b := filepath.Join(tmpDir2, "empty.txt")
	os.WriteFile(file1b, []byte(""), 0444) // Different host permission (0444 vs 0400)

	dir1b := filepath.Join(tmpDir2, "nested/empty/dir")
	os.MkdirAll(dir1b, 0755)

	file2b := filepath.Join(tmpDir2, "exec.sh")
	os.WriteFile(file2b, []byte("#!/bin/sh\necho hi"), 0711) // Different host execute (0711 vs 0777)

	entries2 := map[string]string{
		file1b: "empty.txt",
		dir1b:  "nested/empty/dir",
		file2b: "exec.sh",
	}

	res2, err := CreateLayer(entries2)
	if err != nil {
		t.Fatalf("Failed to create layer 2: %v", err)
	}

	if res1.Digest != res2.Digest {
		t.Errorf("Reproducibility FAILURE: Layers built from identical logical content produced different digests: %s vs %s", res1.Digest, res2.Digest)
	}

	// Verify permissions are normalized inside the tar
	f, _ := os.Open(res1.Path)
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if hdr.Name == "empty.txt" && hdr.Mode != 0644 {
			t.Errorf("Permission normalization failure for empty.txt: expected 0644, got %o", hdr.Mode)
		}
		if hdr.Name == "exec.sh" && hdr.Mode != 0755 {
			t.Errorf("Permission normalization failure for exec.sh: expected 0755, got %o", hdr.Mode)
		}
	}
}
