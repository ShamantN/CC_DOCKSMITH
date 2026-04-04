// Package build — cache.go implements the deterministic build cache engine.
//
// Cache Key Components (MUST all be included for correctness):
//  1. Previous layer digest (or base image manifest digest for the first step)
//  2. The exact instruction text
//  3. Current WORKDIR value (empty string if unset)
//  4. Current ENV state serialized in lexicographically sorted key order
//  5. (COPY only) SHA-256 of each source file's bytes, sorted by path
//
// The cache index is stored as a flat JSON file at:
//   ~/.docksmith/cache/index.json
// It maps cache-key-hex → layer-digest.
//
// A cache hit requires both:
//   (a) a matching entry in the index, AND
//   (b) the layer file to actually exist on disk.
//
// If either condition fails, it is treated as a miss.
package build

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"docksmith/internal/config"
)

// cacheIndexFile is the path to the shared cache index.
func cacheIndexFile() string {
	return filepath.Join(config.CacheDir(), "index.json")
}

// cacheIndex is the in-memory representation of the cache index.
type cacheIndex map[string]string // cacheKeyHex → layerDigest

// loadCacheIndex reads the cache index from disk. Returns empty index on missing file.
func loadCacheIndex() (cacheIndex, error) {
	data, err := os.ReadFile(cacheIndexFile())
	if os.IsNotExist(err) {
		return make(cacheIndex), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cache index: %w", err)
	}
	var idx cacheIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse cache index: %w", err)
	}
	return idx, nil
}

// saveCacheIndex persists the cache index to disk atomically.
func saveCacheIndex(idx cacheIndex) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize cache index: %w", err)
	}
	return os.WriteFile(cacheIndexFile(), data, 0644)
}

// CacheKeyInput contains all the components that feed into a cache key.
type CacheKeyInput struct {
	PreviousDigest string   // digest of the last layer or base image manifest
	Instruction    string   // full instruction text e.g. "RUN pip install -r requirements.txt"
	WorkDir        string   // WORKDIR at time of instruction (empty string if unset)
	EnvState       []string // accumulated ENV vars as "KEY=VALUE" strings
	FilePaths      []string // (COPY only) absolute source file paths for hashing
}

// ComputeCacheKey deterministically hashes all components of CacheKeyInput.
// Returns the hex-encoded SHA-256 of the serialized key material.
func ComputeCacheKey(input CacheKeyInput, fileHashFunc func([]string) (string, error)) (string, error) {
	h := sha256.New()

	// 1. Previous digest
	fmt.Fprintf(h, "prev:%s\n", input.PreviousDigest)

	// 2. Instruction text
	fmt.Fprintf(h, "instruction:%s\n", input.Instruction)

	// 3. WORKDIR (empty string if not set)
	fmt.Fprintf(h, "workdir:%s\n", input.WorkDir)

	// 4. ENV state — sort by key, serialize deterministically
	envSorted := sortedEnv(input.EnvState)
	fmt.Fprintf(h, "env:%s\n", envSorted)

	// 5. File hashes (COPY only)
	if len(input.FilePaths) > 0 {
		filesHash, err := fileHashFunc(input.FilePaths)
		if err != nil {
			return "", fmt.Errorf("hash source files: %w", err)
		}
		fmt.Fprintf(h, "files:%s\n", filesHash)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// sortedEnv serializes env vars in lexicographically sorted KEY order.
// Format: "KEY1=val1;KEY2=val2" — empty string if no envs.
func sortedEnv(envVars []string) string {
	if len(envVars) == 0 {
		return ""
	}
	// Parse into map to deduplicate and sort
	kvMap := make(map[string]string, len(envVars))
	for _, kv := range envVars {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			kvMap[parts[0]] = parts[1]
		}
	}
	keys := make([]string, 0, len(kvMap))
	for k := range kvMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+kvMap[k])
	}
	return strings.Join(pairs, ";")
}

// LookupCache checks if a cache key has a matching layer on disk.
// Returns the layer digest on hit, empty string on miss.
func LookupCache(keyHex string) (string, error) {
	idx, err := loadCacheIndex()
	if err != nil {
		return "", err
	}

	digest, found := idx[keyHex]
	if !found {
		return "", nil
	}

	// Verify the layer file actually exists on disk (Section 5.1)
	layerPath := layerFilePath(digest)
	if _, err := os.Stat(layerPath); os.IsNotExist(err) {
		return "", nil // layer file missing → treat as miss
	}

	return digest, nil
}

// StoreCache records a new keyHex → layerDigest mapping in the cache index.
func StoreCache(keyHex, layerDigest string) error {
	idx, err := loadCacheIndex()
	if err != nil {
		return err
	}
	idx[keyHex] = layerDigest
	return saveCacheIndex(idx)
}

// layerFilePath constructs the path to a layer tar given its digest.
// digest is expected as "sha256:<hex>".
func layerFilePath(digest string) string {
	hex := strings.TrimPrefix(digest, "sha256:")
	return filepath.Join(config.LayersDir(), hex+".tar")
}
