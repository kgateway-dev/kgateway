package admincli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path"

	"github.com/solo-io/go-utils/threadsafe"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmdutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
)

const (
	xdsSnapshotPath = "/snapshots/xds"
	krtSnapshotPath = "/snapshots/krt"
	pprofPath       = "/debug/pprof"
	loggingPath     = "/logging"
	versionPath     = "/version"
)

// Client is a utility for executing requests against the kgateway Admin API
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

// XdsSnapshotCmd returns the cmdutils.Cmd that can be run, and will execute a request against the XDS Snapshot path
func (c *Client) XdsSnapshotCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, xdsSnapshotPath)
}

// KrtSnapshotCmd returns the cmdutils.Cmd that can be run, and will execute a request against the KRT Snapshot path
func (c *Client) KrtSnapshotCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, krtSnapshotPath)
}

// PprofCmd returns the cmdutils.Cmd that can be run, and will execute a request against the Pprof path
func (c *Client) PprofCmd(ctx context.Context, childPath string) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, path.Join(pprofPath, childPath))
}

// LoggingCmd returns the cmdutils.Cmd that can be run, and will execute a request against the Logging path
func (c *Client) LoggingCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, loggingPath)
}

// VersionCmd returns the cmdutils.Cmd that can be run, and will execute a request against the Version path
func (c *Client) VersionCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, versionPath)
}

// Response structure for xds snapshot endpoint
type xdsSnapshotResponse struct {
	// map from node id to resources
	Data  map[string]interface{} `json:"data"`
	Error string                 `json:"error"`
}

// GetXdsSnapshot returns the data that is available at the xds snapshot endpoint
func (c *Client) GetXdsSnapshot(ctx context.Context) (map[string]interface{}, error) {
	var out threadsafe.Buffer

	err := c.XdsSnapshotCmd(ctx).WithStdout(&out).Run().Cause()
	if err != nil {
		return nil, err
	}

	var response xdsSnapshotResponse
	err = json.Unmarshal(out.Bytes(), &response)
	if err != nil {
		return nil, err
	}
	if response.Error != "" {
		return nil, errors.New(response.Error)
	}
	return response.Data, nil
}

// GetKrtSnapshot returns the data that is available at the krt snapshot endpoint
func (c *Client) GetKrtSnapshot(ctx context.Context) (string, error) {
	var out threadsafe.Buffer
	err := c.KrtSnapshotCmd(ctx).WithStdout(&out).Run().Cause()
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// GetPprof returns the data that is available at the pprof endpoint
func (c *Client) GetPprof(ctx context.Context, path string) (string, error) {
	var out threadsafe.Buffer
	err := c.PprofCmd(ctx, path).WithStdout(&out).Run().Cause()
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// GetLogging returns the data that is available at the logging endpoint
func (c *Client) GetLogging(ctx context.Context) (string, error) {
	var out threadsafe.Buffer
	err := c.LoggingCmd(ctx).WithStdout(&out).Run().Cause()
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// GetVersion returns the data that is available at the version endpoint
func (c *Client) GetVersion(ctx context.Context) (map[string]string, error) {
	var out threadsafe.Buffer
	err := c.VersionCmd(ctx).WithStdout(&out).Run().Cause()
	if err != nil {
		return nil, err
	}
	var resp map[string]string
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		return nil, err
	}
	return resp, nil
}
