package build

import (
	"io/fs"
	"path/filepath"
	"time"
)

// snapshotModTimes returns a map of all files in the rootfs relative path 
// to their modification times.
func snapshotModTimes(rootfs string) (map[string]time.Time, error) {
	snapshot := make(map[string]time.Time)
	err := filepath.WalkDir(rootfs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		
		info, err := d.Info()
		if err != nil {
			return err
		}
		
		relPath, _ := filepath.Rel(rootfs, path)
		if relPath == "." {
			return nil
		}
		
		snapshot[relPath] = info.ModTime()
		return nil
	})
	
	return snapshot, err
}

// captureDelta compares the current filesystem against a prior snapshot
// and returns a map mapping absolute source path to relative target path 
// for all new or modified files.
func captureDelta(rootfs string, snapshot map[string]time.Time) (map[string]string, error) {
	entries := make(map[string]string)
	err := filepath.WalkDir(rootfs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		
		info, err := d.Info()
		if err != nil {
			return err
		}
		
		relPath, _ := filepath.Rel(rootfs, path)
		if relPath == "." {
			return nil
		}
		
		prevTime, exists := snapshot[relPath]
		// Determine modified or new
		if !exists || info.ModTime().After(prevTime) {
			// Do not add directories purely because their modTime changed, unless
			// they are new directories or files inside them. For this simple engine,
			// adding a directory just means ensuring it exists, but the files 
			// inside are more important. Wait, we must include it if new.
			
			// We format relative target for tar (e.g. "app/main.go")
			tarPath := relPath
			if d.IsDir() {
				// avoid adding just modTime changed directories to delta layer 
				// to keep layers small, but if it's genuinely new we should.
				if exists {
					return nil
				}
			} else {
                // Ensure symlinks are tracked correctly without attempting to look up their target ModTime
                // (though Lstat is used internally if we used proper Lstat tracking).
			}
			entries[path] = tarPath
		}
		
		return nil
	})
	
	return entries, err
}
