package testutils

import (
	"context"
	"github.com/spf13/cobra"
	"strings"

	cli "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
)

// NewCli returns an implementation of the Cli
func NewCli() *Cli {
	return &Cli{}
}

// Cli is way to execute glooctl commands consistently
type Cli struct{}

// NewCommand returns a fresh cobra.Command
func (c *Cli) NewCommand(ctx context.Context) *cobra.Command {
	// Under the hood we call the cobra.Command directly so that we re-use whatever functionality is available to users
	return cli.CommandWithContext(ctx)
}

// Execute executes an arbitrary glooctl command
func (c *Cli) Execute(ctx context.Context, argStr string) error {
	return ExecuteCommandWithArgs(c.NewCommand(ctx), strings.Split(argStr, " ")...)
}

// ExecuteOut executes an arbitrary glooctl command
func (c *Cli) ExecuteOut(ctx context.Context, argStr string) (string, error) {
	return ExecuteCommandWithArgsOut(c.NewCommand(ctx), strings.Split(argStr, " ")...)
}

// Check attempts to check the installation, and returns an error if one was encountered
func (c *Cli) Check(ctx context.Context, extraArgs ...string) (string, error) {
	checkArgs := append([]string{
		"check",
	}, extraArgs...)

	return ExecuteCommandWithArgsOut(c.NewCommand(ctx), checkArgs...)
}

// CheckCrds attempts to check the CRDs in the cluster, and returns an error if one was encountered
func (c *Cli) CheckCrds(ctx context.Context, extraArgs ...string) error {
	checkCrdArgs := append([]string{
		"check-crds",
	}, extraArgs...)
	return ExecuteCommandWithArgs(c.NewCommand(ctx), checkCrdArgs...)
}

// DebugLogs attempts to output the logs to a specified file
func (c *Cli) DebugLogs(ctx context.Context, extraArgs ...string) error {
	debugLogsArgs := append([]string{
		"debug",
		"logs",
	}, extraArgs...)
	return ExecuteCommandWithArgs(c.NewCommand(ctx), debugLogsArgs...)
}
