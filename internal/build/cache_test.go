package build

import (
	"os"
	"path/filepath"
	"testing"

	"docksmith/internal/config"
)

func setupCacheTest(t *testing.T) string {
	t.Helper()
	tmpDir, _ := os.MkdirTemp("", "docksmith-cache-test-*")
	config.StateDir = tmpDir
	os.MkdirAll(filepath.Join(tmpDir, "cache"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "layers"), 0755)
	return tmpDir
}

// noopFileHash is a test double that returns a fixed hash without touching disk.
func noopFileHash(paths []string) (string, error) {
	return "fakehash", nil
}

func TestComputeCacheKey_Stable(t *testing.T) {
	input := CacheKeyInput{
		PreviousDigest: "sha256:abc123",
		Instruction:    "RUN pip install -r requirements.txt",
		WorkDir:        "/app",
		EnvState:       []string{"PORT=8080", "HOST=0.0.0.0"},
	}

	k1, err := ComputeCacheKey(input, noopFileHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	k2, err := ComputeCacheKey(input, noopFileHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if k1 != k2 {
		t.Errorf("cache key is not stable: %s vs %s", k1, k2)
	}
}

func TestComputeCacheKey_ENVOrderIndependent(t *testing.T) {
	// Same env vars in different order must produce SAME key (sorted by key)
	input1 := CacheKeyInput{
		PreviousDigest: "sha256:abc",
		Instruction:    "RUN echo",
		EnvState:       []string{"Z=1", "A=2"},
	}
	input2 := CacheKeyInput{
		PreviousDigest: "sha256:abc",
		Instruction:    "RUN echo",
		EnvState:       []string{"A=2", "Z=1"},
	}

	k1, _ := ComputeCacheKey(input1, noopFileHash)
	k2, _ := ComputeCacheKey(input2, noopFileHash)
	if k1 != k2 {
		t.Errorf("env order should not matter: %s vs %s", k1, k2)
	}
}

func TestComputeCacheKey_ENVValueChange_BustsCache(t *testing.T) {
	base := CacheKeyInput{
		PreviousDigest: "sha256:abc",
		Instruction:    "RUN echo $PORT",
		WorkDir:        "/app",
		EnvState:       []string{"PORT=8080"},
	}
	changed := CacheKeyInput{
		PreviousDigest: "sha256:abc",
		Instruction:    "RUN echo $PORT",
		WorkDir:        "/app",
		EnvState:       []string{"PORT=9090"}, // different value
	}

	k1, _ := ComputeCacheKey(base, noopFileHash)
	k2, _ := ComputeCacheKey(changed, noopFileHash)
	if k1 == k2 {
		t.Errorf("changing ENV value should bust cache key, but both produced: %s", k1)
	}
}

func TestComputeCacheKey_WorkDirChange_BustsCache(t *testing.T) {
	base := CacheKeyInput{PreviousDigest: "sha256:abc", Instruction: "RUN echo", WorkDir: "/app"}
	changed := CacheKeyInput{PreviousDigest: "sha256:abc", Instruction: "RUN echo", WorkDir: "/other"}

	k1, _ := ComputeCacheKey(base, noopFileHash)
	k2, _ := ComputeCacheKey(changed, noopFileHash)
	if k1 == k2 {
		t.Errorf("WORKDIR change should bust cache key")
	}
}

func TestComputeCacheKey_PrevDigestChange_BustsCache(t *testing.T) {
	base := CacheKeyInput{PreviousDigest: "sha256:aaa", Instruction: "RUN echo", WorkDir: "/"}
	changed := CacheKeyInput{PreviousDigest: "sha256:bbb", Instruction: "RUN echo", WorkDir: "/"}

	k1, _ := ComputeCacheKey(base, noopFileHash)
	k2, _ := ComputeCacheKey(changed, noopFileHash)
	if k1 == k2 {
		t.Errorf("previous digest change should bust cache key")
	}
}

func TestLookupAndStoreCache(t *testing.T) {
	tmpDir := setupCacheTest(t)
	defer os.RemoveAll(tmpDir)

	keyHex := "deadbeef1234"
	layerDigest := "sha256:aabbcc"

	// Nothing in cache yet
	got, err := LookupCache(keyHex)
	if err != nil {
		t.Fatalf("lookup err: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string on miss, got %s", got)
	}

	// Create a fake layer file so lookup can verify it exists
	layerHex := "aabbcc"
	layerPath := filepath.Join(tmpDir, "layers", layerHex+".tar")
	os.WriteFile(layerPath, []byte("fake tar"), 0644)

	// Store then lookup
	if err := StoreCache(keyHex, layerDigest); err != nil {
		t.Fatalf("store err: %v", err)
	}

	got, err = LookupCache(keyHex)
	if err != nil {
		t.Fatalf("lookup err after store: %v", err)
	}
	if got != layerDigest {
		t.Errorf("expected %s, got %s", layerDigest, got)
	}
}

func TestLookupCache_MissingLayerFile_ReturnsMiss(t *testing.T) {
	tmpDir := setupCacheTest(t)
	defer os.RemoveAll(tmpDir)

	// Store entry but do NOT create the layer file
	keyHex := "key123"
	layerDigest := "sha256:orphan"
	_ = StoreCache(keyHex, layerDigest)

	got, err := LookupCache(keyHex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected miss when layer file missing, got: %s", got)
	}
}

func TestSortedEnv(t *testing.T) {
	tests := []struct {
		envVars  []string
		expected string
	}{
		{[]string{}, ""},
		{[]string{"B=2", "A=1"}, "A=1;B=2"},
		{[]string{"A=1", "B=2"}, "A=1;B=2"},
		{[]string{"Z=26", "A=1", "M=13"}, "A=1;M=13;Z=26"},
		// Duplicate key should take the last value
		{[]string{"A=1", "A=2"}, "A=2"},
	}

	for _, tt := range tests {
		got := sortedEnv(tt.envVars)
		if got != tt.expected {
			t.Errorf("sortedEnv(%v) = %q, want %q", tt.envVars, got, tt.expected)
		}
	}
}
