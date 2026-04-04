package build

import (
	"strings"
	"testing"
	"os"
	"path/filepath"
	
	"docksmith/internal/config"
)

func TestParser_ValidInstructions(t *testing.T) {
	// Setup mock config directory so FROM can find the base image
	tmpDir, err := os.MkdirTemp("", "docksmith-parser-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	config.StateDir = tmpDir
	config.SkipIsolationForTesting = true
	os.MkdirAll(filepath.Join(tmpDir, "images"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "layers"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "cache"), 0755)
	
	// Set an empty context dir for testing
	state := NewBuildState()
	state.ContextDir = tmpDir
	
	// Create mock base image
	mockImage := filepath.Join(tmpDir, "images", "alpine_latest.json")
	os.WriteFile(mockImage, []byte("{}"), 0644)

	docksmithfile := `
# This is a comment
FROM alpine:latest

WORKDIR /app
ENV PORT=8080
ENV HOST=0.0.0.0
# overwrite previous
ENV PORT=9000

COPY . /app
RUN echo "Hello world"
CMD ["server", "start"]
`

	state.ContextDir = tmpDir
	executor := NewExecutor(state)
	parser := NewParser(executor)

	err = parser.Parse(strings.NewReader(docksmithfile))
	if err != nil {
		t.Fatalf("unexpected parsing error: %v", err)
	}

	if state.BaseImage != "alpine:latest" {
		t.Errorf("expected BaseImage 'alpine:latest', got '%s'", state.BaseImage)
	}
	if state.Config.WorkingDir != "/app" {
		t.Errorf("expected WorkingDir '/app', got '%s'", state.Config.WorkingDir)
	}
	
	expectedEnv := []string{"PORT=9000", "HOST=0.0.0.0"}
	if len(state.Config.Env) != len(expectedEnv) {
		t.Errorf("expected %d env vars, got %d", len(expectedEnv), len(state.Config.Env))
	}
	for i, e := range state.Config.Env {
		if e != expectedEnv[i] {
			t.Errorf("env mismatch at index %d: expected %s, got %s", i, expectedEnv[i], e)
		}
	}
	
	if len(state.Config.Cmd) != 2 || state.Config.Cmd[0] != "server" || state.Config.Cmd[1] != "start" {
		t.Errorf("expected Cmd ['server', 'start'], got %v", state.Config.Cmd)
	}
}

func TestParser_UnknownInstructions(t *testing.T) {
	// Setup mock config directory
	tmpDir, _ := os.MkdirTemp("", "docksmith-parser-test")
	defer os.RemoveAll(tmpDir)
	config.StateDir = tmpDir
	os.MkdirAll(filepath.Join(tmpDir, "images"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "images", "alpine_latest.json"), []byte("{}"), 0644)

	docksmithfile := `
FROM alpine:latest
EXPOSE 8080
`
	state := NewBuildState()
	executor := NewExecutor(state)
	parser := NewParser(executor)

	err := parser.Parse(strings.NewReader(docksmithfile))
	if err == nil {
		t.Fatal("expected error on unknown instruction EXPOSE, got nil")
	}
	
	if !strings.Contains(err.Error(), "[Error] line 3") || !strings.Contains(err.Error(), "Unknown instruction 'EXPOSE'") {
		t.Errorf("error did not contain expected line number and message, got: %v", err)
	}
}

func TestParser_InvalidCMD(t *testing.T) {
	// Setup mock config directory
	tmpDir, _ := os.MkdirTemp("", "docksmith-parser-test")
	defer os.RemoveAll(tmpDir)
	config.StateDir = tmpDir
	os.MkdirAll(filepath.Join(tmpDir, "images"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "images", "alpine_latest.json"), []byte("{}"), 0644)

	docksmithfile := `
FROM alpine:latest
CMD echo hello
`
	state := NewBuildState()
	executor := NewExecutor(state)
	parser := NewParser(executor)

	err := parser.Parse(strings.NewReader(docksmithfile))
	if err == nil {
		t.Fatal("expected error on invalid nested CMD, got nil")
	}
	if !strings.Contains(err.Error(), "[Error] line 3") || !strings.Contains(err.Error(), "JSON array") {
		t.Errorf("expected JSON array error on line 3, got: %v", err)
	}
}

func TestParser_NoFrom(t *testing.T) {
	docksmithfile := `
WORKDIR /app
CMD ["hello"]
`
	state := NewBuildState()
	executor := NewExecutor(state)
	parser := NewParser(executor)

	err := parser.Parse(strings.NewReader(docksmithfile))
	if err == nil {
		t.Fatal("expected error on missing FROM, got nil")
	}
	if !strings.Contains(err.Error(), "no FROM instruction provided") {
		t.Errorf("expected no FROM error, got: %v", err)
	}
}
