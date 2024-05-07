package cmdutils

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	_ Cmd = &CobraCmd{}
)

func NewCobraCmd(ctx context.Context, command *cobra.Command, args []string) *CobraCmd {
	command.SetArgs(args)
	command.SetContext(ctx)

	return &CobraCmd{
		Command: command,
		args:    args,

		outWriter: io.Discard,
	}
}

// CobraCmd wraps spf13/cobra.Command, implementing the cmdutils.Cmd interface
type CobraCmd struct {
	*cobra.Command
	args []string

	outWriter io.Writer
}

// WithEnv has no effect
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

// WithStderr has no effect
// See Run() for details around why this is not supported at the moment
func (c *CobraCmd) WithStderr(writer io.Writer) Cmd {
	// do nothing
	return c
}

// Run executes the cobra.Command, and returns a RunError if an error occurred during execution
// NOTE:
//
//	cobra.Command's support configuring an alternative to using stdout and stderr
//	However, glooctl does not rely on this functionality and uses os.Stdout directly
//	We opt to bake this complexity directly into this tool, instead of forcing developers to
//	be aware of it. As a result, we do the following:
//		1. Capture the stdout and stderr Files
//		2. Update them to point to a writer of our choosing
//		3. Execute the command
//		4. Undo the change to stdout and stderr
//		5. Write the contents back to whatever writer the caller configured
func (c *CobraCmd) Run() *RunError {
	stdOut := os.Stdout
	stdErr := os.Stderr

	outReader, outWriter, outErr := os.Pipe()
	if outErr != nil {
		return &RunError{
			command:    c.args,
			output:     nil,
			inner:      outErr,
			stackTrace: errors.WithStack(outErr),
		}
	}

	os.Stdout = outWriter
	os.Stderr = outWriter

	outC := make(chan string)

	// copy the output in a separate goroutine so printing can't block indefinitely
	go func() {
		var outBuf bytes.Buffer
		io.Copy(&outBuf, outReader)

		outC <- outBuf.String()
	}()

	err := c.ExecuteContext(c.Context())

	// restoring the real stdout,stderr
	outWriter.Close()
	os.Stdout = stdOut
	os.Stderr = stdErr

	outStr := <-outC

	// We need to report the string back to the writer that the caller had configured
	_, _ = c.outWriter.Write([]byte(outStr))

	if err != nil {
		return &RunError{
			command:    c.args,
			output:     []byte(outStr),
			inner:      err,
			stackTrace: errors.WithStack(err),
		}
	}

	return nil
}
