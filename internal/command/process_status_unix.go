//go:build !windows

package command

import (
	"errors"
	"os/exec"
	"syscall"
)

func platformSignalExitCode(err error) (int, bool) {
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return 0, false
	}
	status, ok := exitError.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() {
		return 0, false
	}
	return 128 + int(status.Signal()), true
}

func normalizeCLIExitCode(code int) int {
	if code <= 0 {
		return 1
	}
	if code > 255 {
		code &= 0xff
		if code == 0 {
			return 1
		}
	}
	return code
}
