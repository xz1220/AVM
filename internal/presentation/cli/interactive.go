package cli

import (
	"errors"
	"os"

	"github.com/charmbracelet/huh"
)

// stdinIsTTY reports whether stdin is attached to a terminal. The CLI
// always treats absent TTY as non-interactive even when --non-interactive
// was not set, so prompts never fail silently in pipes. We avoid pulling
// in go-isatty as a direct dependency; the stdlib FileMode check is
// sufficient on POSIX (stdin in a pipe/file does not carry ModeCharDevice).
func stdinIsTTY() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// isInteractive returns true only when the user can actually answer
// prompts. We require both: --non-interactive must be unset AND stdin
// must be a TTY.
func isInteractive(g globals) bool {
	if g.NonInteractive {
		return false
	}
	return stdinIsTTY()
}

// errNonInteractiveMissing is returned by helpers that bail when a
// required input can only be obtained interactively.
type errNonInteractiveMissing struct {
	Hint string
}

func (e errNonInteractiveMissing) Error() string { return e.Hint }

func newMissingInputErr(hint string) error { return errNonInteractiveMissing{Hint: hint} }

// promptSelect runs huh.NewSelect for a list of string options, with a
// fall-back error when no options are available.
func promptSelect(title string, options []string) (string, error) {
	if len(options) == 0 {
		return "", errors.New("no options")
	}
	opts := make([]huh.Option[string], 0, len(options))
	for _, o := range options {
		opts = append(opts, huh.NewOption(o, o))
	}
	var choice string
	err := huh.NewSelect[string]().
		Title(title).
		Options(opts...).
		Value(&choice).
		Run()
	return choice, err
}

// promptInput prompts for a single line of text with an optional
// default. value is updated in-place.
func promptInput(title string, value *string) error {
	return huh.NewInput().
		Title(title).
		Value(value).
		Run()
}

// promptConfirm asks a yes/no question.
func promptConfirm(title string) (bool, error) {
	var ok bool
	err := huh.NewConfirm().Title(title).Value(&ok).Run()
	return ok, err
}

// promptMultiSelect runs a checkbox-style multi-select returning the
// chosen option values.
func promptMultiSelect(title string, options []string) ([]string, error) {
	if len(options) == 0 {
		return nil, nil
	}
	opts := make([]huh.Option[string], 0, len(options))
	for _, o := range options {
		opts = append(opts, huh.NewOption(o, o))
	}
	var picked []string
	err := huh.NewMultiSelect[string]().
		Title(title).
		Options(opts...).
		Value(&picked).
		Run()
	return picked, err
}
