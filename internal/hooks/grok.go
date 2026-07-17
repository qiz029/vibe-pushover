package hooks

import "fmt"

func grokNotifyCommand(goos, executable, event, pushoverConfig string) (string, error) {
	quote := func(value string) (string, error) {
		return shellQuote(value), nil
	}
	if goos == "windows" {
		quote = windowsShellQuote
	}

	executableArg, err := quote(executable)
	if err != nil {
		return "", fmt.Errorf("quote Grok Build executable: %w", err)
	}
	command := executableArg + " notify --agent grok --event " + event + " --ignore-errors"
	if pushoverConfig == "" {
		return command, nil
	}
	configArg, err := quote(pushoverConfig)
	if err != nil {
		return "", fmt.Errorf("quote Grok Build Pushover config: %w", err)
	}
	return command + " --config " + configArg, nil
}
