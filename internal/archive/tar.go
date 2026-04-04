// Package archive implements the deterministic tar archiver for Docksmith layers.
//
// Reproducibility rules (ALL must be followed to guarantee cache hits across runs):
//
//  1. Tar entries are added in lexicographically sorted path order.
//  2. All timestamps (ModTime, AccessTime, ChangeTime) are zeroed (time.Time{}).
//  3. Uid, Gid are forced to 0; Uname, Gname are forced to "root".
//
// The SHA-256 of the raw tar bytes is the layer's content-address.
// The archiver streams directly to disk while computing the hash simultaneously
// via io.MultiWriter, avoiding any double-buffering of layer data.
package archive

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"docksmith/internal/config"
)

// LayerResult holds the outcome of writing a layer to disk.
type LayerResult struct {
	Digest string // "sha256:<hex>"
	Size   int64  // byte size of the tar on disk
	Path   string // absolute path to the layer file (temp, before rename)
}

// CreateLayer archives the given list of (srcPath → tarPath) mappings into a
// content-addressed layer tar under ~/.docksmith/layers/.
//
// entries maps the source absolute path on disk to its target path inside the tar.
// The resulting tar contains only these files (the delta for this instruction).
func CreateLayer(entries map[string]string) (*LayerResult, error) {
	layersDir := config.LayersDir()

	// Write to a temp file first so we can rename atomically after hashing
	tmpFile, err := os.CreateTemp(layersDir, "layer-*.tar.tmp")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp layer file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file if we error out
	success := false
	defer func() {
		if !success {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	// io.MultiWriter → simultaneously write to disk AND hash
	h := sha256.New()
	mw := io.MultiWriter(tmpFile, h)

	tw := tar.NewWriter(mw)

	// Sort entries deterministically by their tar path
	tarPaths := make([]string, 0, len(entries))
	for _, tarPath := range entries {
		tarPaths = append(tarPaths, tarPath)
	}
	sort.Strings(tarPaths)

	// Build reverse map: tarPath → srcPath
	reverse := make(map[string]string, len(entries))
	for src, tarDst := range entries {
		reverse[tarDst] = src
	}

	for _, tarPath := range tarPaths {
		srcPath := reverse[tarPath]
		if err := addEntry(tw, srcPath, tarPath); err != nil {
			return nil, fmt.Errorf("failed to add %s to tar: %w", tarPath, err)
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize tar: %w", err)
	}

	// Get the file size before closing
	size, err := tmpFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, fmt.Errorf("failed to get layer size: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close temp layer file: %w", err)
	}

	// Compute the digest
	digest := fmt.Sprintf("sha256:%x", h.Sum(nil))

	// Rename to final content-addressed path
	finalPath := filepath.Join(layersDir, fmt.Sprintf("%x.tar", h.Sum(nil)))
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return nil, fmt.Errorf("failed to rename layer file: %w", err)
	}

	success = true
	return &LayerResult{
		Digest: digest,
		Size:   size,
		Path:   finalPath,
	}, nil
}

// addEntry adds a single file or directory entry to the tar writer with
// all non-content metadata normalized for reproducibility.
func addEntry(tw *tar.Writer, srcPath, tarPath string) error {
	info, err := os.Lstat(srcPath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", srcPath, err)
	}

	// Build a normalized header — zero all timestamps and ids
	hdr := &tar.Header{
		Name:     tarPath,
		Mode:     int64(info.Mode()),
		Uid:      0,
		Gid:      0,
		Uname:    "root",
		Gname:    "root",
		ModTime:  time.Time{},
		Size:     0,
		Typeflag: tar.TypeReg,
	}

	if info.IsDir() {
		hdr.Typeflag = tar.TypeDir
		hdr.Name = tarPath + "/"
		hdr.Mode = 0755
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		return nil
	}

	if info.Mode()&fs.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(srcPath)
		if err != nil {
			return fmt.Errorf("readlink %s: %w", srcPath, err)
		}
		hdr.Typeflag = tar.TypeSymlink
		hdr.Linkname = linkTarget
		hdr.Size = 0
		return tw.WriteHeader(hdr)
	}

	// Regular file
	hdr.Size = info.Size()
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", srcPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("write %s to tar: %w", srcPath, err)
	}

	return nil
}

// HashFile returns the SHA-256 hex digest of a file's raw bytes.
// If the path is a directory, it returns a special hash representing a directory.
// Used for COPY cache key computation.
func HashFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return "dir", nil
	}
    // Also skip symlinks for now, or just let them read target if open follows them?
    // standard os.Open follows symlinks.

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// HashFiles returns a combined SHA-256 over all files at the given paths,
// processed in lexicographically sorted order. Used for COPY cache keys.
func HashFiles(paths []string) (string, error) {
	sorted := make([]string, len(paths))
	copy(sorted, paths)
	sort.Strings(sorted)

	h := sha256.New()
	if err := hashFilesInto(h, sorted); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func hashFilesInto(h hash.Hash, paths []string) error {
	for _, p := range paths {
		fh, err := HashFile(p)
		if err != nil {
			return err
		}
		// Include the path itself in the hash so renames bust the cache
		fmt.Fprintf(h, "%s:%s\n", p, fh)
	}
	return nil
}

// ExtractLayer extracts a tar layer file into the given destination directory.
// Later layers naturally overwrite earlier conflicting paths.
func ExtractLayer(layerPath, destDir string) error {
	f, err := os.Open(layerPath)
	if err != nil {
		return fmt.Errorf("open layer %s: %w", layerPath, err)
	}
	defer f.Close()

	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		target := filepath.Join(destDir, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, fs.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("mkdir %s: %w", target, err)
			}
		case tar.TypeReg, 0: // TypeReg = 0x30, also handle 0
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("mkdir for file %s: %w", target, err)
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fs.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("create file %s: %w", target, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return fmt.Errorf("write file %s: %w", target, err)
			}
			out.Close()
		case tar.TypeSymlink:
			os.Remove(target) // remove if already exists
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return fmt.Errorf("symlink %s -> %s: %w", target, hdr.Linkname, err)
			}
		}
	}
	return nil
}
