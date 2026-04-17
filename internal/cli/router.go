package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"docksmith/internal/build"
	"docksmith/internal/config"
	"docksmith/internal/image"
	"docksmith/internal/runtime"
)

// StringSliceFlag implements flag.Value to allow repeatable string flags
type StringSliceFlag []string

func (i *StringSliceFlag) String() string {
	return fmt.Sprintf("%v", *i)
}

func (i *StringSliceFlag) Set(value string) error {
	*i = append(*i, value)
	return nil
}

// Router handles command line parsing and execution routing
type Router struct {
	out io.Writer
	err io.Writer
}

// NewRouter creates a new CLI router
func NewRouter(out, err io.Writer) *Router {
	return &Router{
		out: out,
		err: err,
	}
}

// Execute routes the subcommands from args
func (r *Router) Execute(args []string) int {
	if len(args) < 1 {
		r.printUsage()
		return 1
	}

	command := args[0]
	cmdArgs := args[1:]

	switch command {
	case "build":
		return r.handleBuild(cmdArgs)
	case "images":
		return r.handleImages(cmdArgs)
	case "rmi":
		return r.handleRmi(cmdArgs)
	case "run":
		return r.handleRun(cmdArgs)
	case "internal-child":
		return r.handleChild(cmdArgs)
	default:
		fmt.Fprintf(r.err, "Unknown command: %s\n", command)
		r.printUsage()
		return 1
	}
}

func (r *Router) printUsage() {
	fmt.Fprintf(r.out, "docksmith - A simplified Docker-like build and runtime system\n")
	fmt.Fprintf(r.out, "\nUsage:\n")
	fmt.Fprintf(r.out, "  docksmith <command> [arguments]\n")
	fmt.Fprintf(r.out, "\nCommands:\n")
	fmt.Fprintf(r.out, "  build    Build an image from a Docksmithfile\n")
	fmt.Fprintf(r.out, "  images   List images\n")
	fmt.Fprintf(r.out, "  rmi      Remove an image\n")
	fmt.Fprintf(r.out, "  run      Run a command in a new container\n")
}

func (r *Router) handleBuild(args []string) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(r.err)
	
	tag := fs.String("t", "", "Name and optionally a tag in the 'name:tag' format")
	noCache := fs.Bool("no-cache", false, "Do not use cache when building the image")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *tag == "" {
		fmt.Fprintf(r.err, "Error: -t flag is required\n")
		return 1
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(r.err, "Error: requires context path\n")
		return 1
	}

	contextPath, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(r.err, "Error resolving context path: %v\n", err)
		return 1
	}
	docksmithFilePath := filepath.Join(contextPath, "Docksmithfile")
	
	file, err := os.Open(docksmithFilePath)
	if err != nil {
		fmt.Fprintf(r.err, "Error opening Docksmithfile: %v\n", err)
		return 1
	}
	defer file.Close()

	state := build.NewBuildState()
	state.ContextDir = contextPath
	state.NoCache = *noCache

	// Load existing manifest to preserve timestamp if possible (100% cache hit case)
	name, imgTag := image.ParseNameTag(*tag)
	if existing, err := image.LoadManifest(name, imgTag); err == nil {
		state.OriginalCreated = existing.Created
	}
	
	executor := build.NewExecutor(state)
	parser := build.NewParser(executor)

	fmt.Fprintf(r.out, "Building image '%s' from '%s'\n", *tag, contextPath)
	if err := parser.Parse(file); err != nil {
		fmt.Fprintf(r.err, "Build failed: %v\n", err)
		return 1
	}

	// Build the manifest natively
	// name and imgTag are already parsed at line 119
	
	// Prepare layer entries
	// We need size and createdBy theoretically, but we only have digests in state right now.
	// We can stat the layers to get size.
	var layerEntries []image.LayerEntry
	for i, digest := range state.LayerDigests {
		layerHex := strings.TrimPrefix(digest, "sha256:")
		fileInfo, _ := os.Stat(filepath.Join(config.LayersDir(), layerHex+".tar"))
		size := int64(0)
		if fileInfo != nil {
			size = fileInfo.Size()
		}
		
		layerEntries = append(layerEntries, image.LayerEntry{
			Digest:    digest,
			Size:      size,
			CreatedBy: fmt.Sprintf("Layer %d", i),
		})
	}

	manifest := image.NewManifest(name, imgTag, image.ImageConfig{
		Env:        state.Config.Env,
		Cmd:        state.Config.Cmd,
		WorkingDir: state.Config.WorkingDir,
	}, layerEntries)

	// Preserve original timestamp only if NOT a cache miss (100% cache hit)
	preserveTime := ""
	if !state.CacheCascade && !state.NoCache {
		preserveTime = state.OriginalCreated
	}

	if err := image.SaveManifest(manifest, preserveTime); err != nil {
		fmt.Fprintf(r.err, "Failed to save manifest: %v\n", err)
		return 1
	}

	fmt.Fprintf(r.out, "Successfully built image %s:%s\n", name, imgTag)
	fmt.Fprintf(r.out, "Digest: %s\n", manifest.Digest)
	return 0
}

func (r *Router) handleImages(args []string) int {
	fs := flag.NewFlagSet("images", flag.ContinueOnError)
	fs.SetOutput(r.err)

	if err := fs.Parse(args); err != nil {
		return 1
	}

	entries, err := os.ReadDir(config.ImagesDir())
	if err != nil {
		fmt.Fprintf(r.err, "Error reading images directory: %v\n", err)
		return 1
	}

	fmt.Fprintf(r.out, "%-20s %-15s %-15s %-25s\n", "NAME", "TAG", "IMAGE ID", "CREATED")
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		manifest, err := image.LoadManifestFromPath(filepath.Join(config.ImagesDir(), e.Name()))
		if err != nil {
			continue
		}

		// ID is the first 12 characters of the digest hash (after sha256:)
		imageID := strings.TrimPrefix(manifest.Digest, "sha256:")
		if len(imageID) > 12 {
			imageID = imageID[:12]
		}

		fmt.Fprintf(r.out, "%-20s %-15s %-15s %-25s\n", manifest.Name, manifest.Tag, imageID, manifest.Created)
	}
	return 0
}

func (r *Router) handleRmi(args []string) int {
	fs := flag.NewFlagSet("rmi", flag.ContinueOnError)
	fs.SetOutput(r.err)

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(r.err, "Error: requires image name:tag\n")
		return 1
	}

	nameTag := fs.Arg(0)
	name, tag := image.ParseNameTag(nameTag)

	manifest, err := image.LoadManifest(name, tag)
	if err != nil {
		fmt.Fprintf(r.err, "Error: image not found: %s\n", nameTag)
		return 1
	}

	// 1. Delete manifest file
	manifestPath := image.ManifestPath(name, tag)
	if err := os.Remove(manifestPath); err != nil {
		fmt.Fprintf(r.err, "Error removing manifest: %v\n", err)
		return 1
	}

	// 2. Delete all layers referenced in manifest
	for _, layer := range manifest.Layers {
		layerHex := strings.TrimPrefix(layer.Digest, "sha256:")
		layerPath := filepath.Join(config.LayersDir(), layerHex+".tar")
		_ = os.Remove(layerPath) // Ignore error if layer already gone or shared
	}

	fmt.Fprintf(r.out, "Deleted image: %s\n", nameTag)
	return 0
}

func (r *Router) handleRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(r.err)
	
	var envVars StringSliceFlag
	fs.Var(&envVars, "e", "Set environment variables (can be used multiple times)")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(r.err, "Error: requires image name:tag\n")
		return 1
	}

	rawImage := fs.Arg(0)
	name, tag := image.ParseNameTag(rawImage)

	manifest, err := image.LoadManifest(name, tag)
	if err != nil {
		fmt.Fprintf(r.err, "Error loading image: image not found: %s\n", rawImage)
		return 1
	}

	var cmdArgs []string
	if fs.NArg() > 1 {
		cmdArgs = fs.Args()[1:]
	} else {
		cmdArgs = manifest.Config.Cmd
	}

	if len(cmdArgs) == 0 {
		fmt.Fprintf(r.err, "Error: No command specified and image has no default CMD\n")
		return 1
	}

	// Merge Envs prioritizing flags
	finalEnv := make(map[string]string)
	
	// Load manifest envs
	for _, kv := range manifest.Config.Env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			finalEnv[parts[0]] = parts[1]
		}
	}
	// Override with flags
	for _, kv := range envVars {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			finalEnv[parts[0]] = parts[1]
		}
	}
	
	// Format back to slice
	var mergedEnv []string
	for k, v := range finalEnv {
		mergedEnv = append(mergedEnv, k+"="+v)
	}

	// Assemble rootfs
	rootfs, err := runtime.AssembleRootFS(manifest)
	if err != nil {
		fmt.Fprintf(r.err, "Error assembling filesystem: %v\n", err)
		return 1
	}
	defer os.RemoveAll(rootfs)

	// Execute isolated
	exitCode, err := runtime.ExecuteIsolated(rootfs, cmdArgs, mergedEnv, manifest.Config.WorkingDir)
	if err != nil {
		fmt.Fprintf(r.err, "Execution error: %v\n", err)
		return exitCode
	}

	return 0
}

// Execute is a convenience helper for main.go
func Execute() {
	r := NewRouter(os.Stdout, os.Stderr)
	os.Exit(r.Execute(os.Args[1:]))
}
func (r *Router) handleChild(args []string) int {
	if len(args) < 3 {
		return 1
	}
	rootfs := args[0]
	workDir := args[1]
	// Remaining args are the command to execute
	cmdArgs := args[2:]

	if err := runtime.RunChildProcess(rootfs, workDir, cmdArgs); err != nil {
		fmt.Fprintf(r.err, "Child execution error: %v\n", err)
		return 1
	}
	return 0
}
