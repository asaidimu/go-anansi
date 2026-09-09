package main

import (
	"os"

	"github.com/AlecAivazis/survey/v2"
	"golang.org/x/term"
)

// isInteractive reports whether stdin is a terminal, i.e. interactive prompts
// are safe to show. Piped/CI input falls back to flags and defaults.
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// promptString asks for a single-line value with a default.
func promptString(message, def string, target *string) error {
	return survey.AskOne(&survey.Input{Message: message, Default: def}, target)
}

// promptSelect asks the user to pick one option; the chosen label is written
// to target.
func promptSelect(message string, options []string, target *string) error {
	return survey.AskOne(&survey.Select{Message: message, Options: options}, target)
}
