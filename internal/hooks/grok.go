package hooks

import "fmt"

func grokNotifyCommand(goos, executable, event, pushoverConfig string) (string, error) {
	return hookNotifyCommandForOS(goos, "grok", "Grok Build", executable, event, pushoverConfig)
}

func hookNotifyCommandForOS(goos, agent, displayName, executable, event, pushoverConfig string) (string, error) {
	return hookNotifyCommandForOSWithFlags(goos, agent, displayName, executable, event, pushoverConfig)
}

func hookNotifyCommandForOSWithFlags(goos, agent, displayName, executable, event, pushoverConfig string, flags ...string) (string, error) {
	quote := func(value string) (string, error) {
		return shellQuote(value), nil
	}
	if goos == "windows" {
		quote = windowsShellQuote
	}

	executableArg, err := quote(executable)
	if err != nil {
		return "", fmt.Errorf("quote %s executable: %w", displayName, err)
	}
	command := executableArg + " notify --agent " + agent + " --event " + event + " --ignore-errors"
	for _, flag := range flags {
		command += " " + flag
	}
	if pushoverConfig == "" {
		return command, nil
	}
	configArg, err := quote(pushoverConfig)
	if err != nil {
		return "", fmt.Errorf("quote %s Pushover config: %w", displayName, err)
	}
	return command + " --config " + configArg, nil
}
