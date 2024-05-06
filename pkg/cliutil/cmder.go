package cliutil

import (
	"context"
	"io"

	"github.com/solo-io/gloo/pkg/utils/cmdutils"
	cli "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
)

var (
	_        cmdutils.Cmder = &CliCmder{}
	cliCmder                = &CliCmder{}
)

// Command is a convenience wrapper over cliCmder.Command
func Command(ctx context.Context, args ...string) cmdutils.Cmd {
	return cliCmder.Command(ctx, cli.Name, args...).
		WithStdout(io.Discard).
		WithStderr(io.Discard)
}

// CliCmder is a factory for cmdutils.CobraCmd, implementing cmdutils.Cmder
type CliCmder struct{}

// Command returns a Cmd
func (c *CliCmder) Command(ctx context.Context, _ string, arg ...string) cmdutils.Cmd {
	cmd := cli.CommandWithContext(ctx)
	cmd.SetContext(ctx)
	cmd.SetArgs(arg)

	return &cmdutils.CobraCmd{
		Command: cmd,
	}
}
