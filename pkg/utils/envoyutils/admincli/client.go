package admincli

import (
	"context"
	"github.com/solo-io/gloo/pkg/utils/cmdutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"io"
)

const (
	ConfigDumpPath = "config_dump"
	StatsPath      = "stats"
	ClustersPath   = "clusters"
	ListenersPath  = "listeners"
)

// Client is a utility for executing requests against the Envoy Admin API
type Client struct {
	// receiver is the default destination for the curl stdout and stderr
	receiver io.Writer

	// curlOptions is the set of default Option that the Client will use for curl commands
	curlOptions []curl.Option
}

// NewClient returns an implementation of the admincli.Client
func NewClient(receiver io.Writer, curlOptions []curl.Option) *Client {
	defaultCurlOptions := []curl.Option{
		curl.WithScheme("http"),
		// 5 retries, exponential back-off, 10 second max
		curl.WithRetries(5, 0, 10),
	}

	return &Client{
		receiver:    receiver,
		curlOptions: append(defaultCurlOptions, curlOptions...),
	}
}

func (c *Client) Command(ctx context.Context, options ...curl.Option) cmdutils.Cmd {
	commandCurlOptions := append(
		c.curlOptions,
		// Ensure any options defined for this command can override any defaults that the Client has defined
		options...)
	curlArgs := curl.BuildArgs(ctx, commandCurlOptions...)

	return cmdutils.Command(ctx, "curl", curlArgs...).
		// For convenience, we set the stdout and stderr to the receiver
		// This can still be overwritten by consumers who use the commands
		WithStdout(c.receiver).
		WithStderr(c.receiver)
}

func (c *Client) RunCommand(ctx context.Context, options ...curl.Option) error {
	return c.Command(ctx, options...).Run().Cause()
}

func (c *Client) RequestPathCmd(ctx context.Context, path string) cmdutils.Cmd {
	return c.Command(ctx, curl.WithPath(path))
}

func (c *Client) StatsCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, StatsPath)
}

func (c *Client) ClustersCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, ClustersPath)
}

func (c *Client) ListenersCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, ListenersPath)
}

func (c *Client) ConfigDumpCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, ConfigDumpPath)
}
