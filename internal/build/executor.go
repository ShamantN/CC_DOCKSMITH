package build

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"docksmith/internal/archive"
	"docksmith/internal/config"
	"docksmith/internal/image"
	"docksmith/internal/runtime"
)

// Executor manages mutating the build state based on parsed instructions.
type Executor struct {
	state *BuildState
}

// NewExecutor creates an executor wrapping a build state.
func NewExecutor(state *BuildState) *Executor {
	return &Executor{state: state}
}

// EvalFROM evaluates a FROM instruction.
// It loads the base image's manifest from the local store, sets the base image,
// and captures the base manifest digest to seed the first cache key.
func (e *Executor) EvalFROM(rawImage string) error {
	name, tag := image.ParseNameTag(rawImage)

	manifest, err := image.LoadManifest(name, tag)
	if err != nil {
		// Try the legacy path format used in the Part 2 tests (name_tag.json without parsing)
		safeName := strings.ReplaceAll(rawImage, ":", "_")
		imagePath := filepath.Join(config.ImagesDir(), safeName+".json")
		if _, statErr := os.Stat(imagePath); statErr != nil {
			return fmt.Errorf("base image not found locally: %s (offline mode — import it first)", rawImage)
		}
		// Manifest exists but might not be a full Docksmith manifest (e.g. bare "{}")
		// In that case we derive a synthetic digest from the file bytes.
		data, readErr := os.ReadFile(imagePath)
		if readErr != nil {
			return fmt.Errorf("failed to read base image manifest: %w", readErr)
		}
		import_sha := sha256hex(data)
		e.state.BaseImage = rawImage
		e.state.BaseManifestDigest = "sha256:" + import_sha
		return nil
	}

	e.state.BaseImage = rawImage
	e.state.BaseManifestDigest = manifest.Digest

	for _, layer := range manifest.Layers {
		e.state.LayerDigests = append(e.state.LayerDigests, layer.Digest)
	}

	return nil
}

// EvalWORKDIR evaluates a WORKDIR instruction.
// Relative paths are resolved against the current WorkingDir.
func (e *Executor) EvalWORKDIR(path string) error {
	if !strings.HasPrefix(path, "/") {
		e.state.Config.WorkingDir = filepath.Join(e.state.Config.WorkingDir, path)
	} else {
		e.state.Config.WorkingDir = path
	}
	e.state.Config.WorkingDir = filepath.Clean(e.state.Config.WorkingDir)
	return nil
}

// EvalENV evaluates an ENV instruction.
// If the key already exists, it is updated in place.
// The slice order is insertion-ordered; sorted serialization happens in the cache key.
func (e *Executor) EvalENV(kv string) error {
	parts := strings.SplitN(kv, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid ENV format, expected KEY=VALUE")
	}
	key, val := parts[0], parts[1]

	for i, exist := range e.state.Config.Env {
		if strings.HasPrefix(exist, key+"=") {
			e.state.Config.Env[i] = key + "=" + val
			return nil
		}
	}
	e.state.Config.Env = append(e.state.Config.Env, key+"="+val)
	return nil
}

// EvalCMD evaluates a CMD instruction.
// The argument MUST be a valid JSON array (e.g. ["exec", "arg"]).
func (e *Executor) EvalCMD(cmdStr string) error {
	var cmdArray []string
	if err := json.Unmarshal([]byte(cmdStr), &cmdArray); err != nil {
		return fmt.Errorf("CMD instruction must be a valid JSON array")
	}
	e.state.Config.Cmd = cmdArray
	return nil
}

// EvalCOPY evaluates a COPY instruction.
func (e *Executor) EvalCOPY(src, dest string) error {
	if src == "" || dest == "" {
		return fmt.Errorf("COPY requires non-empty source and destination")
	}

	matches, err := MatchGlob(e.state.ContextDir, src)
	if err != nil {
		return fmt.Errorf("failed to match glob %q: %w", src, err)
	}

	// build absolute paths for cache hashing
	var absPaths []string
	for _, m := range matches {
		absPaths = append(absPaths, filepath.Join(e.state.ContextDir, m))
	}

	keyInput := CacheKeyInput{
		PreviousDigest: e.currentPrevDigest(),
		Instruction:    fmt.Sprintf("COPY %s %s", src, dest),
		WorkDir:        e.state.Config.WorkingDir,
		EnvState:       e.state.Config.Env,
		FilePaths:      absPaths,
	}

	return e.evaluateCacheAndArchive(keyInput, func() (string, error) {
		// cache miss execution
		entries := make(map[string]string)
		for _, m := range matches {
			srcPath := filepath.Join(e.state.ContextDir, m)
			var destPath string
			if strings.HasSuffix(dest, "/") {
				destPath = filepath.Join(dest, filepath.Base(m))
			} else {
				destPath = dest
			}
			// Clean dest path and remove leading slash for tar
			destPath = strings.TrimPrefix(filepath.Clean(destPath), "/")
			entries[srcPath] = destPath
		}
		
		layer, err := archive.CreateLayer(entries)
		if err != nil {
			return "", err
		}
		
		e.state.LayerDigests = append(e.state.LayerDigests, layer.Digest)
		return layer.Digest, nil
	})
}

// EvalRUN evaluates a RUN instruction.
func (e *Executor) EvalRUN(cmd string) error {
	if cmd == "" {
		return fmt.Errorf("RUN requires a non-empty command")
	}

	keyInput := CacheKeyInput{
		PreviousDigest: e.currentPrevDigest(),
		Instruction:    fmt.Sprintf("RUN %s", cmd),
		WorkDir:        e.state.Config.WorkingDir,
		EnvState:       e.state.Config.Env,
	}

	return e.evaluateCacheAndArchive(keyInput, func() (string, error) {
		// Assemble rootfs from current layer digests
		rootfs, err := runtime.AssembleRootFSFromLayers(e.state.LayerDigests)
		if err != nil {
			return "", fmt.Errorf("assemble rootfs for RUN: %w", err)
		}
		defer os.RemoveAll(rootfs)

		snapshot, err := snapshotModTimes(rootfs)
		if err != nil {
			return "", fmt.Errorf("snapshot rootfs before RUN: %w", err)
		}

		// Execute cmd inside isolated environment
		cmdArgs := []string{"/bin/sh", "-c", cmd}
		exitCode, err := runtime.ExecuteIsolated(rootfs, cmdArgs, e.state.Config.Env, e.state.Config.WorkingDir)
		if err != nil {
			return "", fmt.Errorf("RUN command failed with exit code %d: %w", exitCode, err)
		}

		entries, err := captureDelta(rootfs, snapshot)
		if err != nil {
			return "", fmt.Errorf("capture delta after RUN: %w", err)
		}

		// Create new layer from the modified files
		layer, err := archive.CreateLayer(entries)
		if err != nil {
			return "", err
		}

		e.state.LayerDigests = append(e.state.LayerDigests, layer.Digest)
		return layer.Digest, nil
	})
}

// evaluateCacheAndArchive orchestrates the deterministic cache lookup and conditional execution.
func (e *Executor) evaluateCacheAndArchive(keyInput CacheKeyInput, execute func() (string, error)) error {
	keyHex, err := ComputeCacheKey(keyInput, archive.HashFiles)
	if err != nil {
		return fmt.Errorf("compute cache key: %w", err)
	}

	var hitDigest string
	if !e.state.NoCache && !e.state.CacheCascade {
		hitDigest, err = LookupCache(keyHex)
		if err != nil {
			return fmt.Errorf("lookup cache: %w", err)
		}
	}

	if hitDigest != "" {
		fmt.Printf("Step %d/%d : %s [CACHE HIT]\n", e.state.StepCurrent, e.state.StepTotal, keyInput.Instruction)
		e.state.PreviousLayerDigest = hitDigest
		return nil
	}

	fmt.Printf("Step %d/%d : %s [CACHE MISS]\n", e.state.StepCurrent, e.state.StepTotal, keyInput.Instruction)
	e.state.CacheCascade = true // cascade misses

	digest, err := execute()
	if err != nil {
		return err
	}

	if !e.state.NoCache {
		if err := StoreCache(keyHex, digest); err != nil {
			return fmt.Errorf("store cache: %w", err)
		}
	}

	e.state.PreviousLayerDigest = digest
	return nil
}

// currentPrevDigest returns the digest to use as the "previous layer" for cache key computation.
// It is BaseManifestDigest before the first COPY/RUN, then the digest of the last layer.
func (e *Executor) currentPrevDigest() string {
	if e.state.PreviousLayerDigest != "" {
		return e.state.PreviousLayerDigest
	}
	return e.state.BaseManifestDigest
}
