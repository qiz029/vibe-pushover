package command

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

type secretPrompter struct {
	input    io.Reader
	buffered *bufio.Reader
	output   io.Writer
}

func newSecretPrompter(input io.Reader, output io.Writer) *secretPrompter {
	return &secretPrompter{
		input:    input,
		buffered: bufio.NewReader(input),
		output:   output,
	}
}

func (p *secretPrompter) readRequired(label string) (string, error) {
	for {
		value, err := p.readSecret(label)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
		if _, err := fmt.Fprintln(p.output, "Value cannot be empty; please try again."); err != nil {
			return "", err
		}
	}
}

func (p *secretPrompter) readSecret(label string) (string, error) {
	if _, err := fmt.Fprint(p.output, label); err != nil {
		return "", err
	}
	if file, ok := p.input.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		value, err := term.ReadPassword(int(file.Fd()))
		_, newlineErr := fmt.Fprintln(p.output)
		if err != nil {
			return "", fmt.Errorf("read secret: %w", err)
		}
		if newlineErr != nil {
			return "", newlineErr
		}
		return strings.TrimSpace(string(value)), nil
	}

	value, err := p.buffered.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read secret: %w", err)
	}
	value = strings.TrimSpace(value)
	if errors.Is(err, io.EOF) && value == "" {
		return "", errors.New("read secret: unexpected end of input")
	}
	return value, nil
}

func (p *secretPrompter) readChoice(label, defaultValue string, choices ...string) (string, error) {
	allowed := make(map[string]struct{}, len(choices))
	for _, choice := range choices {
		allowed[choice] = struct{}{}
	}
	for {
		if _, err := fmt.Fprint(p.output, label); err != nil {
			return "", err
		}
		value, err := p.buffered.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read choice: %w", err)
		}
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			return defaultValue, nil
		}
		if _, ok := allowed[value]; ok {
			return value, nil
		}
		if _, err := fmt.Fprintf(p.output, "Choose one of: %s.\n", strings.Join(choices, ", ")); err != nil {
			return "", err
		}
	}
}
