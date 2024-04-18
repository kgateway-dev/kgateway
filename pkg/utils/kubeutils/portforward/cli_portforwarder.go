package portforward

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"

	errors "github.com/rotisserie/eris"

	"github.com/avast/retry-go/v4"
)

var _ PortForwarder = &cliPortForwarder{}

// NewCliPortForwarder returns an implementation of a PortForwarder that relies on the Kubernetes CLI to perform port-forwarding
// This implementation is NOT thread-safe
func NewCliPortForwarder(options ...Option) PortForwarder {
	return &cliPortForwarder{
		properties: buildPortForwardProperties(options...),

		// The following are populated when Start is invoked
		errCh:     nil,
		cmd:       nil,
		cmdCancel: nil,
	}
}

type cliPortForwarder struct {
	// properties represents the set of user-defined values to configure the apiPortForwarder
	properties *properties

	errCh chan error

	cmd       *exec.Cmd
	cmdCancel context.CancelFunc
}

func (c *cliPortForwarder) Start(ctx context.Context, options ...retry.Option) error {
	return retry.Do(func() error {
		return c.startOnce(ctx)
	}, options...)
}

func (c *cliPortForwarder) startOnce(ctx context.Context) error {
	if c.properties.localPort == 0 {
		// 0 is a special value, which means "choose for me a free port"
		freePort, err := getFreePort()
		if err != nil {
			return err
		}
		c.properties.localPort = freePort
	}

	cmdCtx, cmdCancel := context.WithCancel(ctx)

	c.cmd = exec.CommandContext(
		cmdCtx,
		"kubectl",
		"port-forward",
		"-n",
		c.properties.resourceNamespace,
		fmt.Sprintf("%s/%s", c.properties.resourceType, c.properties.resourceName),
		fmt.Sprintf("%d:%d", c.properties.localPort, c.properties.remotePort),
	)
	c.cmd.Stdout = c.properties.stdout
	c.cmd.Stderr = c.properties.stderr
	c.cmdCancel = cmdCancel

	c.errCh = make(chan error, 1)

	return c.cmd.Start()
}

func (c *cliPortForwarder) Address() string {
	return net.JoinHostPort(c.properties.localAddress, strconv.Itoa(c.properties.localPort))
}

func (c *cliPortForwarder) Close() {
	if c.cmdCancel != nil {
		c.cmdCancel()
	}
}

func (c *cliPortForwarder) ErrChan() <-chan error {
	// This channel is not functional in the cliPortForwarder implementation
	return c.errCh
}

func (c *cliPortForwarder) WaitForStop() {
	if c.cmd.Process != nil {
		c.errCh <- c.cmd.Wait()
	}
}

func getFreePort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	tcpAddr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.Errorf("Error occurred looking for an open tcp port")
	}
	return tcpAddr.Port, nil
}
