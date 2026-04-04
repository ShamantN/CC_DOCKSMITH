package config

import (
	"os"
	"path/filepath"
)

// resolveHome returns the actual invoking user's home directory.
// When running under sudo, $SUDO_USER is set to the original username,
// so we use /home/$SUDO_USER instead of root's home (/root).
func resolveHome() string {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		// Use the real user's home, not root's
		return filepath.Join("/home", sudoUser)
	}
	return os.Getenv("HOME")
}

// StateDir determines the path where offline image state lives.
var StateDir = filepath.Join(resolveHome(), ".docksmith")

// SkipIsolationForTesting lets tests disable Linux namespace usage
var SkipIsolationForTesting = false

// initStateWithHome points the StateDir to a custom home equivalent and ensures directories
func initStateWithHome(home string) error {
	StateDir = filepath.Join(home, ".docksmith")
	return EnsureDirectories()
}

func ImagesDir() string {
	return filepath.Join(StateDir, "images")
}

func LayersDir() string {
	return filepath.Join(StateDir, "layers")
}

func CacheDir() string {
	return filepath.Join(StateDir, "cache")
}

func EnsureDirectories() error {
	dirs := []string{ImagesDir(), LayersDir(), CacheDir()}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}
