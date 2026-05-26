package cmdutils

import (
	"context"
	"io"
)

// Cmd abstracts over running a command somewhere
type Cmd interface {
	// Run executes the command (like os/exec.Cmd.Run)
	// It returns a *RunError if there is any error, nil otherwise
	Run() *RunError

	// Start starts the command but doesn't block
	// If the returned error is non-nil, it should be of type *RunError
	Start() *RunError

	// Wait waits for the command to finish
	// If the returned error is non-nil, it should be of type *RunError
	Wait() *RunError

	// Output returns the output of the executed command
	Output() []byte

	// WithEnv sets the Env variables for the Cmd
	// Each entry should be of the form "key=value"
	WithEnv(...string) Cmd

	// WithStdin sets the io.Reader used for stdin
	WithStdin(reader io.Reader) Cmd

	// WithStdout sets the io.Writer used for stdout
	WithStdout(io.Writer) Cmd

	// WithStderr sets the io.Reader used for stderr
	WithStderr(io.Writer) Cmd

	PrettyCommand() string
}

// QuietableCmd is an optional extension implemented by Cmd values that can
// suppress the "+ <command>" trace line that is otherwise printed to stderr
// when running under the e2e build tag.
type QuietableCmd interface {
	WithQuiet() Cmd
}

// WithQuiet suppresses command trace output for Cmd implementations that
// support it. For Cmd values that do not implement QuietableCmd, it returns
// the original command unchanged.
func WithQuiet(cmd Cmd) Cmd {
	if quietable, ok := cmd.(QuietableCmd); ok {
		return quietable.WithQuiet()
	}

	return cmd
}

// Cmder abstracts over creating commands
type Cmder interface {
	Command(ctx context.Context, name string, args ...string) Cmd
}
