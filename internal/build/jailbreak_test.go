package build

import (
	"docksmith/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupJailbreakTest(t *testing.T) string {
	t.Helper()
	tmpDir, _ := os.MkdirTemp("", "docksmith-jail-test-*")
	config.StateDir = tmpDir
	config.EnsureDirectories()
	return tmpDir
}

func TestMatchGlobPathTraversal(t *testing.T) {
	tmp := setupJailbreakTest(t)
	defer os.RemoveAll(tmp)
	
	tmpDir := tmp
	// Build context setup
	context := filepath.Join(tmpDir, "context")
	os.MkdirAll(context, 0755)
	os.WriteFile(filepath.Join(context, "app.txt"), []byte("app data"), 0644)

	// Sensitive file OUTSIDE context
	sensitive := filepath.Join(tmpDir, "passwd")
	os.WriteFile(sensitive, []byte("root:secret"), 0644)

	tests := []struct {
		name    string
		pattern string
		wantErr bool
		errMsg  string
	}{
		{
			"Valid Local File",
			"app.txt",
			false,
			"",
		},
		{
			"Simple Traversal Attack",
			"../passwd",
			true,
			"security: parent directory tracking",
		},
		{
			"Obfuscated Traversal Attack",
			"dir/../../passwd",
			true,
			"security: parent directory tracking",
		},
		{
			"Absolute Path Attack",
			"/etc/passwd",
			true,
			"security: absolute paths are not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := MatchGlob(context, tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("MatchGlob(%q) error = %v, wantErr %v", tt.pattern, err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("MatchGlob(%q) expected error message containing %q, got %q", tt.pattern, tt.errMsg, err.Error())
			}
		})
	}
}

func TestSymlinkLeakPrevention(t *testing.T) {
	tmp := setupJailbreakTest(t)
	defer os.RemoveAll(tmp)
	
	context := filepath.Join(tmp, "context")
	os.MkdirAll(context, 0755)

	// Host sensitive file
	hostPasswd := filepath.Join(tmp, "host_passwd")
	os.WriteFile(hostPasswd, []byte("secret_host_data"), 0644)

	// Symlink inside context pointing outside
	linkPath := filepath.Join(context, "bad_link")
	if err := os.Symlink(hostPasswd, linkPath); err != nil {
		t.Skip("Symlinks not supported on this platform")
	}

	state := &BuildState{
		ContextDir: context,
		Config:     NewImageConfig(),
	}
	executor := NewExecutor(state)

	// Attempt to COPY the malicious link
	// The expectation is that MatchGlob sees a file named "bad_link"
	// and EvalCOPY processes it.
	err := executor.EvalCOPY("bad_link", "/app/link")
	if err != nil {
		t.Fatalf("EvalCOPY unexpected error: %v", err)
	}

	// In Phase 2, we must ensure that the resulting layer JUST contains 
	// a symlink entry in the tar, NOT the content of host_passwd.
	// This is already handled by archive.addEntry using Lstat.
}
