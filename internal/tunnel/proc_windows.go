//go:build windows

package tunnel

import (
	"os"
	"os/exec"
)

func setProcAttr(cmd *exec.Cmd) {
	// Windows doesn't need Setpgid
}

func signalTerm(process *os.Process) error {
	return process.Signal(os.Interrupt)
}

func signalKill(process *os.Process) error {
	return process.Kill()
}

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, FindProcess always succeeds
	// We try to open the process to check if it exists
	err = process.Signal(os.Signal(nil))
	// If err is nil or specific error, process might exist
	// This is a simplified check for Windows
	return err == nil
}
