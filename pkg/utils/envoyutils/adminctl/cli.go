package adminctl

import (
	"context"
	"github.com/solo-io/gloo/pkg/utils/cmdutils"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/go-utils/contextutils"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	ConfigDumpPath = "config_dump"
	StatsPath      = "stats"
	ClustersPath   = "clusters"
	ListenersPath  = "listeners"
)

// Cli is a utility for executing `kubectl` commands
type Cli struct {
	// receiver is the default destination for the curl stdout and stderr
	receiver io.Writer

	// requestBuilder is the set of default request properties for the Envoy Admin API
	requestBuilder *testutils.CurlRequestBuilder
}

// NewCli returns an implementation of the adminctl.Cli
func NewCli(receiver io.Writer, address string) *Cli {
	addressParts := strings.Split(address, ":")
	service := addressParts[0]
	port, _ := strconv.Atoi(addressParts[1])

	requestBuilder := testutils.DefaultCurlRequestBuilder().
		WithScheme("http").
		WithService(service).
		WithPort(port).
		// 5 retries, exponential back-off, 10 second max
		WithRetries(5, 0, 10)

	return &Cli{
		receiver:       receiver,
		requestBuilder: requestBuilder,
	}
}

func (c *Cli) Command(ctx context.Context, builder *testutils.CurlRequestBuilder) cmdutils.Cmd {
	args, err := builder.BuildArgs()
	if err != nil {
		// An error is returned here to improve the dev experience, as the CurlRequest that was
		// constructed is known to be invalid
		// Therefore, for developers we error loudly using a DPanic
		contextutils.LoggerFrom(ctx).DPanic(err)
	}

	cmd := cmdutils.Command(ctx, "curl", args...)
	cmd.WithEnv(os.Environ()...)

	// For convenience, we set the stdout and stderr to the receiver
	// This can still be overwritten by consumers who use the commands
	cmd.WithStdout(c.receiver)
	cmd.WithStderr(c.receiver)
	return cmd
}

func (c *Cli) RequestPathCmd(ctx context.Context, path string) cmdutils.Cmd {
	return c.Command(ctx, c.requestBuilder.WithPath(path))
}

func (c *Cli) StatsCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, StatsPath)
}

func (c *Cli) ClustersCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, ClustersPath)
}

func (c *Cli) ListenersCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, ListenersPath)
}

func (c *Cli) ConfigDumpCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, ConfigDumpPath)
}
