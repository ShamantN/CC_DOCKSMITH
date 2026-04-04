package runtime

import (
	"fmt"
	"os"
	"path/filepath"

	"docksmith/internal/archive"
	"docksmith/internal/config"
	"docksmith/internal/image"
)

// AssembleRootFS creates a temporary directory and extracts all layers of the 
// manifest sequentially into it, returning the absolute path to the ready rootfs.
// It is the caller's responsibility to delete the returned directory after use.
func AssembleRootFS(manifest *image.ImageManifest) (string, error) {
	tempRootDir, err := os.MkdirTemp("", "docksmith-rootfs-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp rootfs: %w", err)
	}

	for i, entry := range manifest.Layers {
		layerPath := filepath.Join(config.LayersDir(), trimShaPrefix(entry.Digest)+".tar")
		if err := archive.ExtractLayer(layerPath, tempRootDir); err != nil {
			// clean up immediately on failure
			os.RemoveAll(tempRootDir)
			return "", fmt.Errorf("failed to extract layer %d (%s): %w", i, entry.Digest, err)
		}
	}

	return tempRootDir, nil
}

// AssembleRootFSFromLayers behaves exactly like AssembleRootFS but accepts 
// a slice of digest strings instead of a pre-parsed manifest, which is useful 
// during mid-build delta extractions.
func AssembleRootFSFromLayers(layerDigests []string) (string, error) {
	tempRootDir, err := os.MkdirTemp("", "docksmith-rootfs-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp rootfs: %w", err)
	}

	for i, digest := range layerDigests {
		layerPath := filepath.Join(config.LayersDir(), trimShaPrefix(digest)+".tar")
		if err := archive.ExtractLayer(layerPath, tempRootDir); err != nil {
			os.RemoveAll(tempRootDir)
			return "", fmt.Errorf("failed to extract layer %d (%s): %w", i, digest, err)
		}
	}

	return tempRootDir, nil
}

func trimShaPrefix(digest string) string {
	if len(digest) > 7 && digest[:7] == "sha256:" {
		return digest[7:]
	}
	return digest
}
