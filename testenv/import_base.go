package main

import (
"fmt"
"os"
"path/filepath"
"docksmith/internal/archive"
"docksmith/internal/image"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: import_base <src_dir> <name:tag>")
		os.Exit(1)
	}

	srcDir := os.Args[1]
	nameTag := os.Args[2]
	name, tag := image.ParseNameTag(nameTag)

	// Discover all files inside source directory
	entries := make(map[string]string)
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil { return err }
		if path == srcDir { return nil }
		rel, _ := filepath.Rel(srcDir, path)
		entries[path] = rel
		return nil
	})
	if err != nil { panic(err) }

	fmt.Printf("Creating layer from %d files...\n", len(entries))
	layer, err := archive.CreateLayer(entries)
	if err != nil { panic(err) }

	cfg := image.ImageConfig{
		Env: []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
		Cmd: []string{"/bin/sh"},
		WorkingDir: "/",
	}

	layerEntry := image.LayerEntry{
		Digest: layer.Digest,
		Size: layer.Size,
		CreatedBy: "Base Import",
	}

	manifest := image.NewManifest(name, tag, cfg, []image.LayerEntry{layerEntry})
	if err := image.SaveManifest(manifest, ""); err != nil { panic(err) }

	fmt.Printf("Successfully imported %s:%s -> %s\n", name, tag, manifest.Digest)
}
