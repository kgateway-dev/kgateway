package cmdutils

import (
	"io"

	"github.com/pkg/errors"
	"github.com/solo-io/go-utils/threadsafe"
	"github.com/spf13/cobra"
)

var (
	_ Cmd = &CobraCmd{}
)

func NewCobraCmd(command *cobra.Command, args []string) *CobraCmd {
	var combinedOutput threadsafe.Buffer

	return &CobraCmd{
		Command: command,
		args:    args,

		outWriter:      io.Discard,
		errWriter:      io.Discard,
		combinedOutput: &combinedOutput,
	}
}

// CobraCmd wraps spf13/cobra.Command, implementing the cmdutils.Cmd interface
type CobraCmd struct {
	*cobra.Command
	args []string

	combinedOutput *threadsafe.Buffer
	outWriter      io.Writer
	errWriter      io.Writer
}

func (c *CobraCmd) WithEnv(env ...string) Cmd {
	// do nothing
	return c
}

func (c *CobraCmd) WithStdin(reader io.Reader) Cmd {
	c.SetIn(reader)
	return c
}

func (c *CobraCmd) WithStdout(writer io.Writer) Cmd {
	c.outWriter = writer
	return c
}

func (c *CobraCmd) WithStderr(writer io.Writer) Cmd {
	c.errWriter = writer
	return c
}

func (c *CobraCmd) Run() *RunError {
	c.SetOut(io.MultiWriter(c.outWriter, c.combinedOutput))
	c.SetErr(io.MultiWriter(c.errWriter, c.combinedOutput))

	if err := c.Command.Execute(); err != nil {
		return &RunError{
			command:    c.args,
			output:     c.combinedOutput.Bytes(),
			inner:      err,
			stackTrace: errors.WithStack(err),
		}
	}
	return nil
}
