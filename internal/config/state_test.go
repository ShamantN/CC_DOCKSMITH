package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitStateWithHome(t *testing.T) {
	// Create a temporary "home" directory
	tempHome, err := os.MkdirTemp("", "docksmith-home-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	// Run our initialization logic pointing to our temp home
	err = initStateWithHome(tempHome)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// Verify paths
	expectedStateDir := filepath.Join(tempHome, ".docksmith")
	if StateDir != expectedStateDir {
		t.Errorf("expected StateDir %q, got %q", expectedStateDir, StateDir)
	}

	dirsToVerify := []string{
		StateDir,
		filepath.Join(StateDir, "images"),
		filepath.Join(StateDir, "layers"),
		filepath.Join(StateDir, "cache"),
	}

	for _, d := range dirsToVerify {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("directory %q was not created: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("path %q is not a directory", d)
		}
	}

	// Verify helper functions
	if ImagesDir() != filepath.Join(StateDir, "images") {
		t.Errorf("ImagesDir() mismatch")
	}
	if LayersDir() != filepath.Join(StateDir, "layers") {
		t.Errorf("LayersDir() mismatch")
	}
	if CacheDir() != filepath.Join(StateDir, "cache") {
		t.Errorf("CacheDir() mismatch")
	}
}
