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

// ExecuteIsolated configures namespaces and re-executes the current binary 
// with 'internal-child' to safely enter the container environment.
func ExecuteIsolated(rootfs string, cmdArgs []string, env []string, workDir string) (int, error) {
	if len(cmdArgs) == 0 {
		return 1, fmt.Errorf("no command provided for execution")
	}

	if workDir == "" {
		workDir = "/"
	}

	// Re-execute ourselves with the hidden 'internal-child' subcommand
	args := append([]string{"internal-child", rootfs, workDir}, cmdArgs...)
	cmd := exec.Command("/proc/self/exe", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	// Linux strict namespaces allocation mapping
	if !config.SkipIsolationForTesting {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS,
		}
	} else {
		// Mock isolation for testing
		absWorkDir := filepath.Join(rootfs, workDir)
		os.MkdirAll(absWorkDir, 0755)
		cmd.Dir = absWorkDir
	}

	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("failed to start container process: %w", err)
	}

	err := cmd.Wait()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if ws, ok := exitError.Sys().(syscall.WaitStatus); ok {
				return ws.ExitStatus(), fmt.Errorf("container exited with non-zero code: %d", ws.ExitStatus())
			}
		}
		return 1, fmt.Errorf("container execution failed: %w", err)
	}

	return 0, nil
}

// RunChildProcess is called by the 'internal-child' command to finalize 
// the isolation before executing the user's command.
func RunChildProcess(rootfs, workDir string, cmdArgs []string) error {
	// 1. Isolation: Set a unique hostname for the container
	if err := syscall.Sethostname([]byte("docksmith-container")); err != nil {
		return fmt.Errorf("sethostname: %w", err)
	}

	// 2. Filesystem: Trap the process inside the rootfs
	if err := syscall.Chroot(rootfs); err != nil {
		return fmt.Errorf("chroot: %w", err)
	}

	// Ensure workdir exists inside the container before changing to it
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return fmt.Errorf("mkdir workDir: %w", err)
	}
	if err := os.Chdir(workDir); err != nil {
		return fmt.Errorf("chdir: %w", err)
	}

	// 3. Path Resolution: Find the command inside the container's rootfs
	executable := cmdArgs[0]
	if !filepath.IsAbs(executable) {
		pathEnv := os.Getenv("PATH")
		paths := filepath.SplitList(pathEnv)
		found := false
		for _, p := range paths {
			fullPath := filepath.Join(p, executable)
			if _, err := os.Stat(fullPath); err == nil {
				executable = fullPath
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("executable not found in PATH: %s", cmdArgs[0])
		}
	}

	// 4. Final Handover: Replace this process with the target command
	return syscall.Exec(executable, cmdArgs, os.Environ())
}

