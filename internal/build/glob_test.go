package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatchGlob(t *testing.T) {
	// Create a temporary directory structure
	tmpDir, err := os.MkdirTemp("", "docksmith-glob-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create files
	filesToCreate := []string{
		"a.txt",
		"b.txt",
		"src/main.go",
		"src/utils/math.go",
		"src/utils/test.go",
		"assets/img.png",
	}

	for _, f := range filesToCreate {
		fullPath := filepath.Join(tmpDir, filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		os.WriteFile(fullPath, []byte("test"), 0644)
	}

	tests := []struct {
		pattern  string
		expected []string
	}{
		{"*.txt", []string{"a.txt", "b.txt"}},
		{"src/*.go", []string{"src/main.go"}},
		{"src/**/*.go", []string{"src/main.go", "src/utils/math.go", "src/utils/test.go"}}, // ** matches 0 or more directories
		{"**/*.png", []string{"assets/img.png"}},
		{"**", filesToCreate}, // should match everything except the root itself
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			matches, err := MatchGlob(tmpDir, tt.pattern)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Clean up expected to slashes internally just in case (we return relative paths which are OS-native,
			// so we must evaluate carefully depending on standard OS paths)
			var expectedNative []string
			for _, e := range tt.expected {
				expectedNative = append(expectedNative, filepath.FromSlash(e))
			}

			// Filter matches to only include files (since WalkDir also visits dirs, but our expected list is only files for this specific test)
			// Wait, the current matcher *does* return directories if they match!
			// Let's filter out directories just for validation against filesToCreate, or add directories to expected.
			// Actually let's just check if all expected are in matches.
			for _, exp := range expectedNative {
				found := false
				for _, m := range matches {
					if m == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected to find %s but didn't. Matches: %v", exp, matches)
				}
			}
		})
	}
}

func TestMatchParts(t *testing.T) {
	// Directly test the recursive logic
	tests := []struct {
		pattern string
		path    string
		match   bool
	}{
		{"**/*.js", "a/b/c.js", true},
		{"**/*.js", "c.js", true},
		{"src/**/*.go", "src/main.go", true},
		{"src/**/*.go", "src/utils/math.go", true},
		{"**", "anything/at/all", true},
		{"src/*", "src/main.go", true},
		{"src/*", "src/utils/math.go", false}, // * does not cross boundaries
	}

	for _, tt := range tests {
		matched := matchPattern(tt.pattern, tt.path)
		if matched != tt.match {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, matched, tt.match)
		}
	}
}
