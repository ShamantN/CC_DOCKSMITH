package main

import (
"fmt"
"os"
"path/filepath"
"docksmith/internal/archive"
"docksmith/internal/image"
)

func main() {
// Discover all files inside alpine
entries := make(map[string]string)
filepath.Walk("alpine", func(path string, info os.FileInfo, err error) error {
if path == "alpine" { return nil }
rel, _ := filepath.Rel("alpine", path)
entries[path] = rel
return nil
})

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

manifest := image.NewManifest("alpine", "latest", cfg, []image.LayerEntry{layerEntry})
if err := image.SaveManifest(manifest, ""); err != nil { panic(err) }

fmt.Printf("Successfully imported alpine:latest -> %s\n", manifest.Digest)
}
