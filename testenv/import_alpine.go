package main

// import_alpine.go — properly imports an Alpine minirootfs tar.gz as a Docksmith base image.
//
// Usage: sudo go run testenv/import_alpine.go <path-to-alpine-minirootfs.tar.gz>
//
// Unlike walking an extracted directory (which loses symlink→directory relationships),
// this reads the ORIGINAL tar entries directly, normalizes them (zero timestamps,
// root uid/gid), and writes a deterministic layer tar via archive.CreateLayer logic.

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type LayerEntry struct {
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	CreatedBy string `json:"createdBy"`
}
type ImageConfig struct {
	Env        []string `json:"Env"`
	Cmd        []string `json:"Cmd"`
	WorkingDir string   `json:"WorkingDir"`
}
type ImageManifest struct {
	Name    string      `json:"name"`
	Tag     string      `json:"tag"`
	Digest  string      `json:"digest"`
	Created string      `json:"created"`
	Config  ImageConfig `json:"config"`
	Layers  []LayerEntry `json:"layers"`
}

func docksmithDir() string {
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser != "" {
		return filepath.Join("/home", sudoUser, ".docksmith")
	}
	return filepath.Join(os.Getenv("HOME"), ".docksmith")
}

func main() {
	tarGzPath := "testenv/alpine.tar.gz"
	if len(os.Args) > 1 {
		tarGzPath = os.Args[1]
	}

	stateDir := docksmithDir()
	layersDir := filepath.Join(stateDir, "layers")
	imagesDir := filepath.Join(stateDir, "images")
	os.MkdirAll(layersDir, 0755)
	os.MkdirAll(imagesDir, 0755)

	fmt.Printf("Importing %s into %s\n", tarGzPath, stateDir)

	f, err := os.Open(tarGzPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening tar.gz: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading gzip: %v\n", err)
		os.Exit(1)
	}
	defer gz.Close()

	// Read all entries from the original tar into memory (headers + content offsets)
	type entry struct {
		hdr  *tar.Header
		data []byte
	}
	var entries []entry

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading tar: %v\n", err)
			os.Exit(1)
		}

		// Normalize for determinism
		hdr.ModTime = time.Time{}
		hdr.AccessTime = time.Time{}
		hdr.ChangeTime = time.Time{}
		hdr.Uid = 0
		hdr.Gid = 0
		hdr.Uname = "root"
		hdr.Gname = "root"
		// Preserve original name, size, typeflag, mode, linkname

		var data []byte
		if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == 0 {
			data, err = io.ReadAll(tr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading entry %s: %v\n", hdr.Name, err)
				os.Exit(1)
			}
			hdr.Size = int64(len(data))
		}

		entries = append(entries, entry{hdr: hdr, data: data})
	}

	// Sort by name for determinism
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].hdr.Name < entries[j].hdr.Name
	})

	// Write deterministic tar to a temp file, hashing simultaneously
	tmpFile, err := os.CreateTemp(layersDir, "layer-*.tar.tmp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating temp layer: %v\n", err)
		os.Exit(1)
	}
	tmpPath := tmpFile.Name()

	h := sha256.New()
	mw := io.MultiWriter(tmpFile, h)
	tw := tar.NewWriter(mw)

	for _, e := range entries {
		if err := tw.WriteHeader(e.hdr); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing header %s: %v\n", e.hdr.Name, err)
			os.Exit(1)
		}
		if len(e.data) > 0 {
			if _, err := tw.Write(e.data); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing data for %s: %v\n", e.hdr.Name, err)
				os.Exit(1)
			}
		}
	}
	if err := tw.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Error finalizing tar: %v\n", err)
		os.Exit(1)
	}

	// Get size
	size, _ := tmpFile.Seek(0, io.SeekCurrent)
	tmpFile.Close()

	// Content-addressed final name
	digest := fmt.Sprintf("sha256:%x", h.Sum(nil))
	hexOnly := fmt.Sprintf("%x", h.Sum(nil))
	finalPath := filepath.Join(layersDir, hexOnly+".tar")

	if err := os.Rename(tmpPath, finalPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error renaming layer: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Layer: %s (%d bytes)\n", digest, size)

	// Build and save manifest
	manifest := ImageManifest{
		Name:    "alpine",
		Tag:     "latest",
		Digest:  "",
		Created: "2024-01-01T00:00:00Z",
		Config: ImageConfig{
			Env:        []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
			Cmd:        []string{"/bin/sh"},
			WorkingDir: "/",
		},
		Layers: []LayerEntry{
			{Digest: digest, Size: size, CreatedBy: "Alpine minirootfs import"},
		},
	}

	// Compute manifest digest (zero out digest field first)
	manifest.Digest = ""
	raw, _ := json.Marshal(manifest)
	msum := sha256.Sum256(raw)
	manifest.Digest = fmt.Sprintf("sha256:%x", msum)

	out, _ := json.MarshalIndent(manifest, "", "  ")
	manifestPath := filepath.Join(imagesDir, "alpine_latest.json")
	if err := os.WriteFile(manifestPath, out, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing manifest: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully imported alpine:latest\nManifest digest: %s\n", manifest.Digest)
	fmt.Printf("Manifest: %s\n", manifestPath)
}
