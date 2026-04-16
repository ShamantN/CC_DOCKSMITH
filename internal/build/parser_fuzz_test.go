package build

import (
	"docksmith/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupParserTest(t *testing.T) string {
	t.Helper()
	tmpDir, _ := os.MkdirTemp("", "docksmith-jail-test-*")
	config.StateDir = tmpDir
	config.EnsureDirectories()
	
	// Create a mock alpine manifest to satisfy FROM checks
	os.WriteFile(filepath.Join(config.ImagesDir(), "alpine_latest.json"), []byte(`{"digest":"sha256:alpine","name":"alpine","tag":"latest"}`), 0644)
	
	// Crucial: Initialize all subdirectories
	config.EnsureDirectories()
	return tmpDir
}

func TestParserFuzz(t *testing.T) {
	tmp := setupParserTest(t)
	defer os.RemoveAll(tmp)

	state := NewBuildState()
	executor := NewExecutor(state)
	parser := NewParser(executor)

	tests := []struct {
		name    string
		content string
		wantErr bool
		errMsg  string
	}{
		{
			"Mixed Case and Extra Whitespace",
			"fRoM   alpine:latest\ncOpY  .   /app\nRUN ls -l\n",
			false,
			"",
		},
		{
			"Multiple Equals in ENV",
			"FROM alpine\nENV KEY=abc=123==\n",
			false,
			"",
		},
		{
			"Missing Arguments FROM",
			"FROM \n",
			true,
			"FROM requires an image argument",
		},
		{
			"Missing Arguments COPY",
			"FROM alpine\nCOPY .\n",
			true,
			"COPY requires source and destination arguments",
		},
		{
			"Unknown Instruction",
			"FROM alpine\nGIBBERISH hello\n",
			true,
			"Unknown instruction 'GIBBERISH'",
		},
		{
			"Comments and Empty Lines",
			"\n# This is a comment\n\nFROM alpine\n\n# Another comment\nRUN date\n",
			false,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset state for each test
			state.BaseImage = ""
			state.Config = NewImageConfig()
			state.CurrentLine = 0

			err := parser.Parse(strings.NewReader(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Parse() expected error message containing %q, got %q", tt.errMsg, err.Error())
			}
		})
	}
}
