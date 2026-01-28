//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func stopProcess(pid int, force bool) {
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}

	if force {
		process.Signal(syscall.SIGKILL)
	} else {
		process.Signal(syscall.SIGTERM)

		done := make(chan struct{})
		go func() {
			process.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			process.Signal(syscall.SIGKILL)
		}
	}
}

func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func getSignals() []os.Signal {
	return []os.Signal{syscall.SIGTERM, syscall.SIGINT}
}
