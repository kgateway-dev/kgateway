package portforward

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"sync"

	"github.com/avast/retry-go/v4"
)

var _ PortForwarder = &cliPortForwarder{}

// NewCliPortForwarder returns an implementation of a PortForwarder that relies on the Kubernetes CLI to perform port-forwarding
func NewCliPortForwarder(options ...Option) PortForwarder {
	return &cliPortForwarder{
		properties: buildPortForwardProperties(options...),

		// The following are populated when Start is invoked
		errCh: nil,
		cmd:   nil,
	}
}

type cliPortForwarder struct {
	// properties represents the set of user-defined values to configure the apiPortForwarder
	properties *properties

	errCh chan error

	sync.RWMutex
	cmd *exec.Cmd
}

func (c *cliPortForwarder) Start(ctx context.Context, options ...retry.Option) error {
	return retry.Do(func() error {
		return c.startOnce(ctx)
	}, options...)
}

func (c *cliPortForwarder) startOnce(ctx context.Context) error {
	var startErr error

	c.useCmdSafe(func() {
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

		c.errCh = make(chan error, 1)

		startErr = c.cmd.Start()
	})

	return startErr
}

func (c *cliPortForwarder) Address() string {
	return net.JoinHostPort(c.properties.localAddress, strconv.Itoa(c.properties.localPort))
}

func (c *cliPortForwarder) Close() {
	// Close invokes process.release() which is considered a Write operation, so we must use a Lock
	c.useCmdSafe(func() {
		if c.cmd.Process != nil {
			c.errCh <- c.cmd.Process.Kill()
		}
	})
}

func (c *cliPortForwarder) ErrChan() <-chan error {
	// This channel is not functional in the cliPortForwarder implementation
	return c.errCh
}

func (c *cliPortForwarder) WaitForStop() {
	c.useCmdSafe(func() {
		if c.cmd.Process != nil {
			c.errCh <- c.cmd.Wait()
		}
	})
}

func (c *cliPortForwarder) useCmdSafe(fn func()) {
	c.Lock()
	defer c.Unlock()
	fn()
}
