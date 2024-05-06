package cliutil

import (
	"context"
	"io"

	"github.com/solo-io/gloo/pkg/utils/cmdutils"
	cli "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
)

var (
	cliCmder = NewCli()
)

// Command is a convenience wrapper over Cli.Command
func Command(ctx context.Context, args ...string) cmdutils.Cmd {
	return cliCmder.Command(ctx, args...).
		WithStdout(io.Discard).
		WithStderr(io.Discard)
}

// NewCli returns an implementation of the kubectl.Cli
func NewCli() *Cli {
	return &Cli{
		receiver: io.Discard,
	}
}

// Cli is a factory for cmdutils.CobraCmd, implementing cmdutils.Cmder
type Cli struct {
	// receiver is the default destination for the glooctl stdout and stderr
	receiver io.Writer
}

// Command returns a Cmd
func (c *Cli) Command(ctx context.Context, arg ...string) cmdutils.Cmd {
	// Under the hood we call the cobra.Command directly so that we re-use whatever functionality
	// is available to users
	cmd := cli.CommandWithContext(ctx)
	cmd.SetContext(ctx)
	cmd.SetArgs(arg)

	return &cmdutils.CobraCmd{
		Command: cmd,
	}
}

// RunCommand builds a Cmd and runs it
func (c *Cli) RunCommand(ctx context.Context, arg ...string) error {
	return c.Command(ctx, arg...).Run().Cause()
}
