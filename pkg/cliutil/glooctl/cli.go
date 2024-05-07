package glooctl

import (
	"context"
	"github.com/solo-io/go-utils/threadsafe"
	"io"

	"github.com/solo-io/gloo/pkg/utils/cmdutils"
	cli "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
)

// NewCli returns an implementation of the Cli
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

// WithReceiver sets the io.Writer that will be used by default for the stdout and stderr
// of cmdutils.Cmd created by the Cli
func (c *Cli) WithReceiver(receiver io.Writer) *Cli {
	c.receiver = receiver
	return c
}

// Command returns a Cmd
func (c *Cli) Command(ctx context.Context, arg ...string) cmdutils.Cmd {
	// Under the hood we call the cobra.Command directly so that we re-use whatever functionality
	// is available to users
	cobraCommand := cli.CommandWithContext(ctx)
	cobraCommand.SetContext(ctx)
	cobraCommand.SetArgs(arg)

	// For convenience, we set the stdout and stderr to the receiver
	// This can still be overwritten by consumers who use the commands
	return cmdutils.NewCobraCmd(cobraCommand, arg).
		WithStderr(c.receiver).
		WithStdout(c.receiver)
}

// RunCommand builds a Cmd and runs it
func (c *Cli) RunCommand(ctx context.Context, arg ...string) error {
	return c.Command(ctx, arg...).Run().Cause()
}

// Check attempts to check the installation, and returns an error if one was encountered
func (c *Cli) Check(ctx context.Context, extraArgs ...string) (string, error) {
	checkArgs := append([]string{
		"check",
	}, extraArgs...)

	var outLocation threadsafe.Buffer

	runErr := c.Command(ctx, checkArgs...).WithStdout(&outLocation).Run().Cause()
	return outLocation.String(), runErr
}

// CheckCrds attempts to check the CRDs in the cluster, and returns an error if one was encountered
func (c *Cli) CheckCrds(ctx context.Context, extraArgs ...string) error {
	checkCrdArgs := append([]string{
		"check-crds",
	}, extraArgs...)
	return c.RunCommand(ctx, checkCrdArgs...)
}

// DebugLogs attempts to output the logs to a specified file
func (c *Cli) DebugLogs(ctx context.Context, extraArgs ...string) error {
	debugLogsArgs := append([]string{
		"debug",
		"logs",
	}, extraArgs...)
	return c.RunCommand(ctx, debugLogsArgs...)
}
