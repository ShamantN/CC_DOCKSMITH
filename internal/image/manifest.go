// Package image implements the image manifest format for Docksmith.
// Every built image is stored as a JSON manifest in ~/.docksmith/images/.
//
// Manifest Digest Calculation (CRITICAL for reproducibility):
//   1. Serialize the manifest with the Digest field set to ""
//   2. SHA-256 the resulting bytes
//   3. Rewrite the manifest with Digest = "sha256:<hash>"
//
// This ensures the on-disk digest is always a hash of the canonical form,
// not a self-referential hash.
package image

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"docksmith/internal/config"
)

// LayerEntry describes a single immutable layer within a manifest.
type LayerEntry struct {
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	CreatedBy string `json:"createdBy"`
}

// ImageConfig matches the runtime configuration embedded in the manifest.
type ImageConfig struct {
	Env        []string `json:"Env"`
	Cmd        []string `json:"Cmd"`
	WorkingDir string   `json:"WorkingDir"`
}

// ImageManifest is the complete on-disk representation of a built image.
type ImageManifest struct {
	Name    string      `json:"name"`
	Tag     string      `json:"tag"`
	Digest  string      `json:"digest"`
	Created string      `json:"created"`
	Config  ImageConfig `json:"config"`
	Layers  []LayerEntry `json:"layers"`
}

// NewManifest creates a fresh manifest with the current timestamp.
func NewManifest(name, tag string, config ImageConfig, layers []LayerEntry) *ImageManifest {
	return &ImageManifest{
		Name:    name,
		Tag:     tag,
		Digest:  "",
		Created: time.Now().UTC().Format(time.RFC3339),
		Config:  config,
		Layers:  layers,
	}
}

// ComputeAndSetDigest serializes the manifest with Digest="" to get the
// canonical form, SHA-256s that, then sets Digest = "sha256:<hash>".
// Returns the computed digest string.
func (m *ImageManifest) ComputeAndSetDigest() (string, error) {
	// Step 1: zero out the digest field
	original := m.Digest
	m.Digest = ""

	// Step 2: serialize deterministically
	data, err := json.Marshal(m)
	if err != nil {
		m.Digest = original
		return "", fmt.Errorf("failed to serialize manifest for digest: %w", err)
	}

	// Step 3: compute SHA-256
	sum := sha256.Sum256(data)
	digest := fmt.Sprintf("sha256:%x", sum)

	// Step 4: set the final digest
	m.Digest = digest
	return digest, nil
}

// ManifestPath returns the file path where this manifest is stored.
// Format: ~/.docksmith/images/<name>_<tag>.json
func ManifestPath(name, tag string) string {
	safeName := strings.ReplaceAll(name+":"+tag, ":", "_")
	return filepath.Join(config.ImagesDir(), safeName+".json")
}

// SaveManifest computes the digest and writes the manifest JSON to disk.
// If preserveCreated is non-empty, it overrides the Created timestamp
// (used for fully-cached rebuilds to preserve the original timestamp).
func SaveManifest(m *ImageManifest, preserveCreated string) error {
	if preserveCreated != "" {
		m.Created = preserveCreated
	}

	if _, err := m.ComputeAndSetDigest(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize manifest: %w", err)
	}

	path := ManifestPath(m.Name, m.Tag)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write manifest to %s: %w", path, err)
	}

	return nil
}

// LoadManifestFromPath reads and parses a manifest JSON file from a specific path.
func LoadManifestFromPath(path string) (*ImageManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var m ImageManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &m, nil
}

// LoadManifest reads and parses a manifest JSON file from disk by name:tag.
func LoadManifest(name, tag string) (*ImageManifest, error) {
	path := ManifestPath(name, tag)
	return LoadManifestFromPath(path)
}

// ParseNameTag splits "name:tag" into its components. Defaults tag to "latest".
func ParseNameTag(nameTag string) (name, tag string) {
	parts := strings.SplitN(nameTag, ":", 2)
	name = parts[0]
	tag = "latest"
	if len(parts) == 2 && parts[1] != "" {
		tag = parts[1]
	}
	return
}
