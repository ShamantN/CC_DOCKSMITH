package build

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// MatchGlob implements custom globbing that supports `*` and `**`.
// It evaluates paths strictly within the root directory (build context).
// It returns a list of matched relative paths.
func MatchGlob(root, pattern string) ([]string, error) {
	var matches []string

	// Convert to OS-specific path for safe joining
	cleanedPattern := filepath.Clean(pattern)
	
	// If no glob pattern exists, just verify it exists exactly
	if !strings.Contains(cleanedPattern, "*") {
		fullPath := filepath.Join(root, cleanedPattern)
		if _, err := os.Stat(fullPath); err == nil {
			return []string{cleanedPattern}, nil
		}
		return nil, os.ErrNotExist
	}

	// Transform to forward slashes for generic matching
	patternSlash := filepath.ToSlash(cleanedPattern)

	// Walk the root directory and evaluate each path
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		relPathSlash := filepath.ToSlash(relPath)
		if matchPattern(patternSlash, relPathSlash) {
			matches = append(matches, relPath)
		}
		
		return nil
	})

	return matches, err
}

// matchPattern attempts to match a path against a glob with ** support
func matchPattern(pattern, path string) bool {
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")
	
	return matchParts(patternParts, pathParts)
}

func matchParts(patternParts []string, pathParts []string) bool {
	pIdx, tIdx := 0, 0
	for pIdx < len(patternParts) && tIdx < len(pathParts) {
		if patternParts[pIdx] == "**" {
			// If ** is the last segment, it matches everything remaining
			if pIdx == len(patternParts)-1 {
				return true
			}
			
			// Otherwise try consuming varying amounts of pathParts
			nextPattern := patternParts[pIdx+1:]
			for i := tIdx; i <= len(pathParts); i++ {
				if matchParts(nextPattern, pathParts[i:]) {
					return true
				}
			}
			return false
		}
		
		// Standard filepath.Match for single segments (handles *)
		matched, err := filepath.Match(patternParts[pIdx], pathParts[tIdx])
		if err != nil || !matched {
			return false
		}
		pIdx++
		tIdx++
	}
	
	// Exact match successfully consumed both
	if pIdx == len(patternParts) && tIdx == len(pathParts) {
		return true
	}
	
	// If path is exhausted but pattern has trailing **
	if tIdx == len(pathParts) {
		for i := pIdx; i < len(patternParts); i++ {
			if patternParts[i] != "**" {
				return false
			}
		}
		return true
	}
	
	return false
}
