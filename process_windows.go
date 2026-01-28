//go:build windows

package main

import (
	"os"
	"os/exec"
	"time"
)

func stopProcess(pid int, force bool) {
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}

	if force {
		process.Kill()
	} else {
		// Windows doesn't have SIGTERM, try graceful then force
		process.Signal(os.Interrupt)

		done := make(chan struct{})
		go func() {
			process.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			process.Kill()
		}
	}
}

func setProcAttr(cmd *exec.Cmd) {
	// Windows doesn't need Setpgid
}

func getSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
