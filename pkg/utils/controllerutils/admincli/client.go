package admincli

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/solo-io/go-utils/threadsafe"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmdutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
)

const (
	InputSnapshotPath = "/snapshots/input"
	xdsSnapshotPath   = "/snapshots/xds"
	krtSnapshotPath   = "/snapshots/krt"
)

// Client is a utility for executing requests against the Gloo Admin API
// The Admin API handlers can be found at: /projects/gloo/pkg/servers/admin
type Client struct {
	// receiver is the default destination for the curl stdout and stderr
	receiver io.Writer

	// curlOptions is the set of default Option that the Client will use for curl commands
	curlOptions []curl.Option
}

// NewClient returns an implementation of the admincli.Client
func NewClient() *Client {
	return &Client{
		receiver: io.Discard,
		curlOptions: []curl.Option{
			curl.WithScheme("http"),
			curl.WithHost("127.0.0.1"),
			curl.WithPort(int(wellknown.KgatewayAdminPort)),
			// 3 retries, exponential back-off, 10 second max
			curl.WithRetries(3, 0, 10),
		},
	}
}

// WithReceiver sets the io.Writer that will be used by default for the stdout and stderr
// of cmdutils.Cmd created by the Client
func (c *Client) WithReceiver(receiver io.Writer) *Client {
	c.receiver = receiver
	return c
}

// WithCurlOptions sets the default set of curl.Option that will be used by default with
// the cmdutils.Cmd created by the Client
func (c *Client) WithCurlOptions(options ...curl.Option) *Client {
	c.curlOptions = append(c.curlOptions, options...)
	return c
}

// Command returns a curl Command, using the provided curl.Option as well as the client.curlOptions
func (c *Client) Command(ctx context.Context, options ...curl.Option) cmdutils.Cmd {
	commandCurlOptions := append(
		c.curlOptions,
		// Ensure any options defined for this command can override any defaults that the Client has defined
		options...)
	curlArgs := curl.BuildArgs(commandCurlOptions...)

	return cmdutils.Command(ctx, "curl", curlArgs...).
		// For convenience, we set the stdout and stderr to the receiver
		// This can still be overwritten by consumers who use the commands
		WithStdout(c.receiver).
		WithStderr(c.receiver)
}

// RunCommand executes a curl Command, using the provided curl.Option as well as the client.curlOptions
func (c *Client) RunCommand(ctx context.Context, options ...curl.Option) error {
	return c.Command(ctx, options...).Run().Cause()
}

// RequestPathCmd returns the cmdutils.Cmd that can be run, and will execute a request against the provided path
func (c *Client) RequestPathCmd(ctx context.Context, path string) cmdutils.Cmd {
	return c.Command(ctx, curl.WithPath(path))
}

// InputSnapshotCmd returns the cmdutils.Cmd that can be run, and will execute a request against the Input Snapshot path
func (c *Client) InputSnapshotCmd(ctx context.Context) cmdutils.Cmd {
	return c.Command(ctx, curl.WithPath(InputSnapshotPath))
}

// XdsSnapshotCmd returns the cmdutils.Cmd that can be run, and will execute a request against the XDS Snapshot path
func (c *Client) XdsSnapshotCmd(ctx context.Context) cmdutils.Cmd {
	return c.Command(ctx, curl.WithPath(xdsSnapshotPath))
}

// KrtSnapshotCmd returns the cmdutils.Cmd that can be run, and will execute a request against the KRT Snapshot path
func (c *Client) KrtSnapshotCmd(ctx context.Context) cmdutils.Cmd {
	return c.Command(ctx, curl.WithPath(krtSnapshotPath))
}

// GetInputSnapshot returns the data that is available at the input snapshot endpoint
func (c *Client) GetInputSnapshot(ctx context.Context) ([]interface{}, error) {
	var outLocation threadsafe.Buffer

	err := c.InputSnapshotCmd(ctx).WithStdout(&outLocation).Run().Cause()
	if err != nil {
		return nil, err
	}

	type anon struct {
		Data  []interface{} `json:"data"`
		Error string        `json:"error"`
	}
	var output anon

	err = json.Unmarshal(outLocation.Bytes(), &output)
	if err != nil {
		return nil, err
	}
	if output.Error != "" {
		return nil, errors.New(output.Error)
	}
	return output.Data, nil
}
