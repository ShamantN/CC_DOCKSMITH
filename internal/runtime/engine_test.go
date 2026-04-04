package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	
	"docksmith/internal/config"
)

func TestAssembleRootFS(t *testing.T) {
	// Skip advanced isolation logic mock checks here
	config.SkipIsolationForTesting = true
	
	// Create mock layers
	tmpState, _ := os.MkdirTemp("", "docksmith-state-*")
	defer os.RemoveAll(tmpState)
	
	config.StateDir = tmpState
	os.MkdirAll(filepath.Join(tmpState, "layers"), 0755)
	
	layerFile := filepath.Join(tmpState, "layers", "testlayer.tar")
	os.WriteFile(layerFile, []byte("invalid tar data"), 0644) // Just needs to exist for Assemble failure checking easily
	
	_, err := AssembleRootFSFromLayers([]string{"sha256:testlayer"})
	if err == nil {
		t.Fatalf("expected failure extracting invalid tar, got nil")
	}
	if !strings.Contains(err.Error(), "extract layer") {
		t.Errorf("unexpected error format: %v", err)
	}
}

func TestExecuteIsolated_Success(t *testing.T) {
	config.SkipIsolationForTesting = true
	
	rootfs, _ := os.MkdirTemp("", "docksmith-rootfs-*")
	defer os.RemoveAll(rootfs)
	
	// echo successfully
	cmd := []string{"echo", "hello docksmith"}
	exitCode, err := ExecuteIsolated(rootfs, cmd, []string{"FOO=BAR"}, "/app")
	if err != nil || exitCode != 0 {
		t.Fatalf("expected success, got code %d err %v", exitCode, err)
	}
}

func TestExecuteIsolated_Failure(t *testing.T) {
	config.SkipIsolationForTesting = true
	
	rootfs, _ := os.MkdirTemp("", "docksmith-rootfs-*")
	defer os.RemoveAll(rootfs)
	
	// command doesn't exist
	cmd := []string{"invalid_command_that_does_not_exist"}
	exitCode, err := ExecuteIsolated(rootfs, cmd, []string{}, "/")
	if err == nil {
		t.Fatalf("expected failure, got nil")
	}
	if exitCode == 0 {
		t.Errorf("expected non-zero exit code")
	}
}
