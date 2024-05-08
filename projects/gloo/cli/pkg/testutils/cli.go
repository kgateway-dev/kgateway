package testutils

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	cli "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
)

// NewGlooCli returns an implementation of the GlooCli
func NewGlooCli() *GlooCli {
	return &GlooCli{}
}

// GlooCli is way to execute glooctl commands consistently
// It has the benefit of invoking the underlying *cobra.Command directly,
// meaning that we do not rely on any generating binaries
type GlooCli struct{}

// NewCommand returns a fresh cobra.Command
func (c *GlooCli) NewCommand(ctx context.Context) *cobra.Command {
	// Under the hood we call the cobra.Command directly so that we re-use whatever functionality is available to users
	return cli.CommandWithContext(ctx)
}

// Execute executes an arbitrary glooctl command
func (c *GlooCli) Execute(ctx context.Context, argStr string) error {
	return ExecuteCommandWithArgs(c.NewCommand(ctx), strings.Split(argStr, " ")...)
}

// ExecuteOut executes an arbitrary glooctl command
// It returns a string containing the output that is piped to stdout and stdrr
// It optionally returns an error if one was encountered
func (c *GlooCli) ExecuteOut(ctx context.Context, argStr string) (string, error) {
	return ExecuteCommandWithArgsOut(c.NewCommand(ctx), strings.Split(argStr, " ")...)
}

// Check attempts to check the installation
// It returns a string containing the output that a user would see if they invoked `glooctl check`
// It optionally returns an error if one was encountered
func (c *GlooCli) Check(ctx context.Context, extraArgs ...string) (string, error) {
	checkArgs := append([]string{
		"check",
	}, extraArgs...)

	return ExecuteCommandWithArgsOut(c.NewCommand(ctx), checkArgs...)
}

// CheckCrds attempts to check the CRDs in the cluster, and returns an error if one was encountered
func (c *GlooCli) CheckCrds(ctx context.Context, extraArgs ...string) error {
	checkCrdArgs := append([]string{
		"check-crds",
	}, extraArgs...)
	return ExecuteCommandWithArgs(c.NewCommand(ctx), checkCrdArgs...)
}

// DebugLogs attempts to output the logs to a specified file, and returns an error if one was encountered
func (c *GlooCli) DebugLogs(ctx context.Context, extraArgs ...string) error {
	debugLogsArgs := append([]string{
		"debug",
		"logs",
	}, extraArgs...)
	return ExecuteCommandWithArgs(c.NewCommand(ctx), debugLogsArgs...)
}
