package kubectl

import (
	"bytes"
	"context"

	"github.com/solo-io/gloo/pkg/utils/cmdutils"

	"io"
	"os"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/portforward"
)

// Cli is a utility for executing `kubectl` commands
type Cli struct {
	// receiver is the default destination for the kubectl stdout and stderr
	receiver io.Writer

	// kubeContext is the optional value of the context for a given kubernetes cluster
	// If it is not supplied, no context will be included in the command
	kubeContext string
}

// NewCli returns an implementation of the kubectl.Cli
func NewCli(receiver io.Writer) *Cli {
	return &Cli{
		receiver:    receiver,
		kubeContext: "",
	}
}

func (c *Cli) WithKubeContext(kubeContext string) *Cli {
	c.kubeContext = kubeContext
	return c
}

func (c *Cli) Command(ctx context.Context, args ...string) cmdutils.Cmd {
	if c.kubeContext != "" {
		args = append([]string{"--context", c.kubeContext}, args...)
	}

	cmd := cmdutils.Command(ctx, "kubectl", args...)
	cmd.WithEnv(os.Environ()...)

	// For convenience, we set the stdout and stderr to the receiver
	// This can still be overwritten by consumers who use the commands
	cmd.WithStdout(c.receiver)
	cmd.WithStderr(c.receiver)
	return cmd
}

func (c *Cli) ExecuteCommand(ctx context.Context, args ...string) error {
	return c.Command(ctx, args...).Run().Cause()
}

func (c *Cli) ApplyCmd(ctx context.Context, content []byte, extraArgs ...string) cmdutils.Cmd {
	args := append([]string{"apply"}, extraArgs...)

	cmd := c.Command(ctx, args...)
	cmd.WithStdin(bytes.NewBuffer(content))
	return cmd
}

func (c *Cli) Apply(ctx context.Context, content []byte, extraArgs ...string) error {
	applyArgs := append([]string{"-f", "-"}, extraArgs...)
	return c.ApplyCmd(ctx, content, applyArgs...).Run().Cause()
}

func (c *Cli) deleteCmd(ctx context.Context, content []byte, extraArgs ...string) cmdutils.Cmd {
	args := append([]string{"delete"}, extraArgs...)

	cmd := c.Command(ctx, args...)
	cmd.WithStdin(bytes.NewBuffer(content))
	return cmd
}

func (c *Cli) Delete(ctx context.Context, content []byte, extraArgs ...string) error {
	deleteYamlArgs := append([]string{"-f", "-"}, extraArgs...)
	return c.deleteCmd(ctx, content, deleteYamlArgs...).Run().Cause()
}

func (c *Cli) copyCmd(ctx context.Context, from, to string) cmdutils.Cmd {
	return c.Command(ctx, "cp", from, to)
}

func (c *Cli) Copy(ctx context.Context, from, to string) error {
	return c.copyCmd(ctx, from, to).Run().Cause()
}

func (c *Cli) StartPortForward(ctx context.Context, options ...portforward.Option) (portforward.PortForwarder, error) {
	options = append([]portforward.Option{
		// We define some default values, which users can then override
		portforward.WithWriters(c.receiver, c.receiver),
		portforward.WithKubeContext(c.kubeContext),
	}, options...)

	portForwarder := portforward.NewCliPortForwarder(options...)
	err := portForwarder.Start(
		ctx,
		retry.LastErrorOnly(true),
		retry.Delay(100*time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
		retry.Attempts(5),
	)
	return portForwarder, err
}
