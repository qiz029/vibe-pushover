//go:build windows

package command

func platformSignalExitCode(error) (int, bool) {
	return 0, false
}

func normalizeCLIExitCode(code int) int {
	if code <= 0 {
		return 1
	}
	return code
}
