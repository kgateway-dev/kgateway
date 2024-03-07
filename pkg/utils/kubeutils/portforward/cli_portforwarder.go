package portforward

import (
	"context"
	"fmt"
	"github.com/avast/retry-go/v4"
	"net"
	"os/exec"
	"strconv"
)

var _ PortForwarder = &cliPortForwarder{}

// NewCliPortForwarder returns an implementation of a PortForwarder that relies on the Kubernetes CLI to perform port-forwarding
func NewCliPortForwarder(options ...Option) PortForwarder {
	return &cliPortForwarder{
		properties: buildPortForwardProperties(options...),

		// The following are populated when Start is invoked
		errCh: make(chan error),
		cmd:   nil,
	}
}

type cliPortForwarder struct {
	// properties represents the set of user-defined values to configure the apiPortForwarder
	properties *properties

	errCh chan error

	cmd *exec.Cmd
}

func (c *cliPortForwarder) Start(ctx context.Context, options ...retry.Option) error {
	return retry.Do(func() error {
		return c.startOnce(ctx)
	}, options...)
}

func (c *cliPortForwarder) startOnce(ctx context.Context) error {
	c.cmd = exec.CommandContext(
		ctx,
		"kubectl",
		"port-forward",
		"-n",
		c.properties.resourceNamespace,
		fmt.Sprintf("%s/%s", c.properties.resourceType, c.properties.resourceName),
		fmt.Sprintf("%d:%d", c.properties.localPort, c.properties.remotePort),
	)
	c.cmd.Stdout = c.properties.stdout
	c.cmd.Stderr = c.properties.stderr

	return c.cmd.Start()
}

func (c *cliPortForwarder) Address() string {
	return net.JoinHostPort(c.properties.localAddress, strconv.Itoa(c.properties.localPort))
}

func (c *cliPortForwarder) Close() {
	if c.cmd.Process != nil {
		c.errCh <- c.cmd.Process.Kill()
		c.errCh <- c.cmd.Process.Release()
	}
}

func (c *cliPortForwarder) ErrChan() <-chan error {
	// This channel is not functional in the cliPortForwarder implementation
	return c.errCh
}

func (c *cliPortForwarder) WaitForStop() {
	c.errCh <- c.cmd.Wait()
}
