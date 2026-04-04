package build

// ImageConfig represents the configuration block of a built image manifest
type ImageConfig struct {
	Env        []string `json:"Env"`
	Cmd        []string `json:"Cmd"`
	WorkingDir string   `json:"WorkingDir"`
}

// NewImageConfig initializes a fresh configuration with default settings
func NewImageConfig() *ImageConfig {
	return &ImageConfig{
		Env:        []string{},
		Cmd:        []string{},
		WorkingDir: "/",
	}
}

// BuildState tracks the execution state while parsing and running a Docksmithfile.
type BuildState struct {
	BaseImage string
	Config    *ImageConfig

	// CurrentLine is the line number currently being parsed (for error messages).
	CurrentLine int

	// BaseManifestDigest is the digest loaded from the FROM image manifest.
	// It seeds the cache key for the very first layer-producing step.
	BaseManifestDigest string

	// PreviousLayerDigest is updated after each COPY/RUN step and feeds into
	// the next step's cache key, creating a chain of dependencies.
	PreviousLayerDigest string

	// CacheCascade is set to true once any step is a cache miss.
	// Once true, ALL subsequent steps are treated as misses (no lookup).
	CacheCascade bool

	// NoCache disables all cache lookups and writes when true (--no-cache flag).
	NoCache bool

	// StepTotal is the total number of instructions (set by parser before execution).
	StepTotal int

	// StepCurrent counts which step number we are at (for build output).
	StepCurrent int

	// LayerDigests tracks all the layers accumulated so far mapping the assembled state
	LayerDigests []string

	// ContextDir is the build context path (host filesystem directory).
	ContextDir string
}

// NewBuildState creates a new empty state object
func NewBuildState() *BuildState {
	return &BuildState{
		Config: NewImageConfig(),
	}
}
