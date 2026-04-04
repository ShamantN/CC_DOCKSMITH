package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"docksmith/internal/config"
)

// IsolationEngine encapsulates the low-level OS mechanisms to run processes 
// inside temporary filesystem roots safely isolated across namespaces.
type IsolationEngine struct{}

// ExecuteIsolated configures namespaces, a chroot, and binds execution streams
// directly executing the target slice isolated from the host machine.
// Note: CLONE_NEWPID and Chroot require elevated capabilities (sudo).
func ExecuteIsolated(rootfs string, cmdArgs []string, env []string, workDir string) (int, error) {
	if len(cmdArgs) == 0 {
		return 1, fmt.Errorf("no command provided for execution")
	}

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	// Process Working Directory is relative to the *inside* of the chroot
	if workDir == "" {
		workDir = "/"
	}
	
	absWorkDir := filepath.Join(rootfs, workDir)
	if err := os.MkdirAll(absWorkDir, 0755); err != nil {
		return 1, fmt.Errorf("failed to ensure workdir exists: %w", err)
	}
	
	cmd.Dir = workDir

	// Linux strict namespaces allocation mapping
	if !config.SkipIsolationForTesting {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS,
			Chroot:     rootfs,
		}
	} else {
		// Without chroot, act like it ran in the folder
		cmd.Dir = absWorkDir
	}

	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("failed to start container process: %w", err)
	}

	// Wait captures the clean exit blocking safely
	err := cmd.Wait()

	if err != nil {
		// Attempt to extract raw boolean exit-code if it was an explicit exit
		if exitError, ok := err.(*exec.ExitError); ok {
			if ws, ok := exitError.Sys().(syscall.WaitStatus); ok {
				return ws.ExitStatus(), fmt.Errorf("container exited with non-zero code: %d", ws.ExitStatus())
			}
		}
		return 1, fmt.Errorf("container execution failed: %w", err)
	}

	return 0, nil
}
