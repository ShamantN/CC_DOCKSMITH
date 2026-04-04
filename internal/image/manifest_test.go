package image

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"docksmith/internal/config"
)

func setup(t *testing.T) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "docksmith-manifest-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	config.StateDir = tmpDir
	os.MkdirAll(filepath.Join(tmpDir, "images"), 0755)
	return tmpDir
}

func TestManifestDigest_IsStable(t *testing.T) {
	tmpDir := setup(t)
	defer os.RemoveAll(tmpDir)

	m := NewManifest("myapp", "latest",
		ImageConfig{Env: []string{"FOO=bar"}, Cmd: []string{"sh"}, WorkingDir: "/app"},
		[]LayerEntry{
			{Digest: "sha256:aaabbb", Size: 1024, CreatedBy: "COPY . /app"},
		},
	)

	d1, err := m.ComputeAndSetDigest()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	d2, err := m.ComputeAndSetDigest()
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if d1 != d2 {
		t.Errorf("digest is not stable: first=%s, second=%s", d1, d2)
	}
	if !strings.HasPrefix(d1, "sha256:") {
		t.Errorf("digest should start with 'sha256:', got: %s", d1)
	}
}

func TestManifestDigest_ChangesWithContent(t *testing.T) {
	tmpDir := setup(t)
	defer os.RemoveAll(tmpDir)

	m1 := NewManifest("app", "v1",
		ImageConfig{Env: []string{"A=1"}, Cmd: []string{"sh"}, WorkingDir: "/"},
		[]LayerEntry{{Digest: "sha256:aaa", Size: 100, CreatedBy: "RUN echo"}},
	)
	m2 := NewManifest("app", "v1",
		ImageConfig{Env: []string{"A=2"}, Cmd: []string{"sh"}, WorkingDir: "/"},
		[]LayerEntry{{Digest: "sha256:aaa", Size: 100, CreatedBy: "RUN echo"}},
	)

	// Fix the Created timestamp so it doesn't differ
	m1.Created = "2024-01-01T00:00:00Z"
	m2.Created = "2024-01-01T00:00:00Z"

	d1, _ := m1.ComputeAndSetDigest()
	d2, _ := m2.ComputeAndSetDigest()

	if d1 == d2 {
		t.Errorf("expected different digests for different content, both got: %s", d1)
	}
}

func TestSaveAndLoadManifest(t *testing.T) {
	tmpDir := setup(t)
	defer os.RemoveAll(tmpDir)

	m := NewManifest("webapp", "v2",
		ImageConfig{
			Env:        []string{"PORT=8080", "HOST=0.0.0.0"},
			Cmd:        []string{"python", "main.py"},
			WorkingDir: "/app",
		},
		[]LayerEntry{
			{Digest: "sha256:abc123", Size: 2048, CreatedBy: "COPY . /app"},
			{Digest: "sha256:def456", Size: 4096, CreatedBy: "RUN pip install -r requirements.txt"},
		},
	)
	m.Created = "2024-01-01T00:00:00Z"

	if err := SaveManifest(m, ""); err != nil {
		t.Fatalf("SaveManifest failed: %v", err)
	}

	loaded, err := LoadManifest("webapp", "v2")
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	if loaded.Name != "webapp" || loaded.Tag != "v2" {
		t.Errorf("name/tag mismatch")
	}
	if loaded.Config.WorkingDir != "/app" {
		t.Errorf("WorkingDir mismatch")
	}
	if len(loaded.Layers) != 2 {
		t.Errorf("expected 2 layers, got %d", len(loaded.Layers))
	}
	if !strings.HasPrefix(loaded.Digest, "sha256:") {
		t.Errorf("expected sha256 digest, got %s", loaded.Digest)
	}
}

func TestSaveManifest_PreservesTimestamp(t *testing.T) {
	tmpDir := setup(t)
	defer os.RemoveAll(tmpDir)

	m := NewManifest("app", "latest",
		ImageConfig{},
		nil,
	)
	m.Created = "2024-01-01T00:00:00Z"

	if err := SaveManifest(m, "2023-06-15T12:00:00Z"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, _ := LoadManifest("app", "latest")
	if loaded.Created != "2023-06-15T12:00:00Z" {
		t.Errorf("expected preserved timestamp, got %s", loaded.Created)
	}
}

func TestParseNameTag(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantTag  string
	}{
		{"alpine:3.18", "alpine", "3.18"},
		{"myapp:latest", "myapp", "latest"},
		{"myapp", "myapp", "latest"},
		{"myapp:", "myapp", "latest"},
	}
	for _, tt := range tests {
		name, tag := ParseNameTag(tt.input)
		if name != tt.wantName || tag != tt.wantTag {
			t.Errorf("ParseNameTag(%q) = (%q, %q), want (%q, %q)",
				tt.input, name, tag, tt.wantName, tt.wantTag)
		}
	}
}
