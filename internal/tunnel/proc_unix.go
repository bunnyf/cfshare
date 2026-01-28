//go:build !windows

package tunnel

import (
	"os"
	"os/exec"
	"syscall"
)

func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func signalTerm(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}

func signalKill(process *os.Process) error {
	return process.Signal(syscall.SIGKILL)
}

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
