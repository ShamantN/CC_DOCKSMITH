package cli

import (
	"bytes"
	"strings"
	"testing"
	"docksmith/internal/config"
)

func TestRouter_Build(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	r := NewRouter(out, errOut)
	config.SkipIsolationForTesting = true

	// Missing context
	code := r.Execute([]string{"build", "-t", "myapp:latest"})
	if code == 0 {
		t.Errorf("Expected failure for missing context, got 0")
	}

	// Missing tag
	code = r.Execute([]string{"build", "."})
	if code == 0 {
		t.Errorf("Expected failure for missing tag, got 0")
	}

	// With tag and context but no Docksmithfile — should fail with an error about Docksmithfile
	code = r.Execute([]string{"build", "-t", "myapp:latest", "--no-cache", "."})
	// We expect failure because there is no Docksmithfile in "."
	// But flag parsing was correct, so it should NOT fail with flag errors.
	if code == 0 {
		// If somehow it succeeds (e.g. a Docksmithfile exists in cwd during tests), that's also fine
		if !strings.Contains(out.String(), "Successfully built") {
			t.Errorf("Unexpected success without output: %s", out.String())
		}
	} else {
		// Should fail with Docksmithfile error, not a flag error
		if !strings.Contains(errOut.String(), "Docksmithfile") && !strings.Contains(errOut.String(), "Build failed") {
			t.Errorf("Expected Docksmithfile error, got: %s", errOut.String())
		}
	}
}

func TestRouter_Run(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	r := NewRouter(out, errOut)
	config.SkipIsolationForTesting = true

	// Missing image
	code := r.Execute([]string{"run"})
	if code == 0 {
		t.Errorf("Expected failure for missing image, got 0")
	}

	// With envs and cmd, but mocked missing image will fail cleanly
	code = r.Execute([]string{"run", "-e", "FOO=BAR", "-e", "BAZ=QUX", "myapp:latest", "echo", "hello"})
	if code == 0 {
		t.Errorf("Expected failure for missing image, got %d", code)
	}

	expectedSubstr := "Error loading image: image not found: myapp:latest"
	if !strings.Contains(errOut.String(), expectedSubstr) {
		t.Errorf("Unexpected output. Expected to contain: %s, Got: %s", expectedSubstr, errOut.String())
	}
}

func TestRouter_Rmi(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	r := NewRouter(out, errOut)
	config.SkipIsolationForTesting = true

	// Missing image
	code := r.Execute([]string{"rmi"})
	if code == 0 {
		t.Errorf("Expected failure for missing image, got 0")
	}

	// Success case requires image to exist, so here we just test proper argument parsing.
	// The real lookup will fail cleanly because no image manifest exists in tmp dir.
	code = r.Execute([]string{"rmi", "myapp:latest"})
	// Expecting error (image not found) - not 0
	if code == 0 {
		t.Logf("Note: rmi succeeded (image might exist from a prior test)")
	} else if !strings.Contains(errOut.String(), "not found") && !strings.Contains(errOut.String(), "removing") {
		t.Errorf("Expected 'not found' in rmi error output, got: %s", errOut.String())
	}
}

func TestRouter_Images(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	r := NewRouter(out, errOut)
	config.SkipIsolationForTesting = true

	code := r.Execute([]string{"images"})
	if code != 0 {
		t.Errorf("Expected success, got %d. Stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "IMAGE") {
		t.Errorf("Unexpected output: %s", out.String())
	}
}
