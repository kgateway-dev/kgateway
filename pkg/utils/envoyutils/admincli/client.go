package admincli

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"

	adminv3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/solo-io/go-utils/threadsafe"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmdutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/portforward"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/protoutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"

	// import for side effects; this is needed so we can unmarshal envoy types
	_ "github.com/kgateway-dev/kgateway/v2/pkg/utils/filter_types"
)

const (
	ConfigDumpPath     = "config_dump"
	StatsPath          = "stats"
	ClustersPath       = "clusters"
	ListenersPath      = "listeners"
	ModifyRuntimePath  = "runtime_modify"
	ShutdownServerPath = "quitquitquit"
	HealthCheckPath    = "healthcheck"
	LoggingPath        = "logging"
	ServerInfoPath     = "server_info"
)

// DumpOptions should have flags for any kind of underlying optional
// filtering or inclusion of Envoy dump data, such as including EDS, filters, etc.
type DumpOptions struct {
	ConfigIncludeEDS bool
}

// Client is a utility for executing requests against the Envoy Admin API
// The Admin API handlers can be found here:
// https://github.com/envoyproxy/envoy/blob/63bc9b564b1a76a22a0d029bcac35abeffff2a61/source/server/admin/admin.cc#L127
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
			curl.WithPort(int(wellknown.EnvoyAdminPort)),
			// 3 retries, exponential back-off, 10 second max
			curl.WithRetries(3, 0, 10),
		},
	}
}

// NewPortForwardedClient takes a pod selector like <podname> or `deployment/<podname`,
// and returns a port-forwarded Envoy admin client pointing at that pod,
// as well as a deferrable shutdown function.
//
// Designed to be used by tests and CLI from outside of a cluster where `kubectl` is present.
// In all other cases, `NewClient` is preferred
func NewPortForwardedClient(ctx context.Context, proxySelector, namespace string) (*Client, func(), error) {
	selector := portforward.WithResourceSelector(proxySelector, namespace)

	// 1. Open a port-forward to the Kubernetes Deployment, so that we can query the Envoy Admin API directly
	portForwarder, err := kubectl.NewCli().StartPortForward(ctx,
		selector,
		portforward.WithRemotePort(int(wellknown.EnvoyAdminPort)))
	if err != nil {
		return nil, nil, err
	}

	// 2. Close the port-forward when we're done accessing data
	deferFunc := func() {
		portForwarder.Close()
		portForwarder.WaitForStop()
	}

	// 3. Create a CLI that connects to the Envoy Admin API
	adminCli := NewClient().
		WithCurlOptions(
			curl.WithHostPort(portForwarder.Address()),
		)

	return adminCli, deferFunc, err
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

// StatsCmd returns the cmdutils.Cmd that can be run to request data from the stats endpoint
func (c *Client) StatsCmd(ctx context.Context, queryParams map[string]string) cmdutils.Cmd {
	return c.Command(ctx,
		curl.WithPath(StatsPath),
		curl.WithQueryParameters(queryParams),
	)
}

// GetStats returns the data that is available at the stats endpoint
func (c *Client) GetStats(ctx context.Context, queryParams map[string]string) (string, error) {
	var outLocation threadsafe.Buffer

	err := c.StatsCmd(ctx, queryParams).WithStdout(&outLocation).Run().Cause()
	if err != nil {
		return "", err
	}

	return outLocation.String(), nil
}

// ServerInfoCmd returns the cmdutils.Cmd that can be run to request data from the server_info endpoint
func (c *Client) ServerInfoCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, ServerInfoPath)
}

// ClustersCmd returns the cmdutils.Cmd that can be run to request data from the clusters endpoint
func (c *Client) ClustersCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, ClustersPath)
}

// ListenersCmd returns the cmdutils.Cmd that can be run to request data from the listeners endpoint
func (c *Client) ListenersCmd(ctx context.Context) cmdutils.Cmd {
	return c.RequestPathCmd(ctx, ListenersPath)
}

// ConfigDumpCmd returns the cmdutils.Cmd that can be run to request data from the config_dump endpoint
func (c *Client) ConfigDumpCmd(ctx context.Context, queryParams map[string]string) cmdutils.Cmd {
	return c.Command(ctx,
		curl.WithPath(ConfigDumpPath),
		curl.WithQueryParameters(queryParams),
	)
}

// GetConfigDump returns the structured data that is available at the config_dump endpoint
func (c *Client) GetConfigDump(ctx context.Context, queryParams map[string]string) (*adminv3.ConfigDump, error) {
	var (
		cfgDump     adminv3.ConfigDump
		outLocation threadsafe.Buffer
	)

	err := c.ConfigDumpCmd(ctx, queryParams).WithStdout(&outLocation).Run().Cause()
	if err != nil {
		return nil, err
	}

	// Ever since upgrading the go-control-plane to v0.10.1 the standard unmarshal fails with the following error:
	// unknown field \"hidden_envoy_deprecated_build_version\" in envoy.config.core.v3.Node"
	// To get around this, we rely on an unmarshaler with AllowUnknownFields set to true
	if err = protoutils.UnmarshalAllowUnknown(&outLocation, &cfgDump); err != nil {
		return nil, err
	}

	return &cfgDump, nil
}

// GetStaticClusters returns the map of static clusters available on a ConfigDump, indexed by their name
func (c *Client) GetStaticClusters(ctx context.Context) (map[string]*clusterv3.Cluster, error) {
	configDump, err := c.GetConfigDump(ctx, map[string]string{
		"resource": "static_clusters",
	})
	if err != nil {
		return nil, err
	}

	return GetStaticClustersByName(configDump)
}

// ModifyRuntimeConfiguration passes the queryParameters to the runtime_modify endpoint
func (c *Client) ModifyRuntimeConfiguration(ctx context.Context, queryParameters map[string]string) error {
	return c.RunCommand(ctx,
		curl.WithPath(ModifyRuntimePath),
		curl.WithQueryParameters(queryParameters),
		curl.WithMethod(http.MethodPost))
}

// ShutdownServer calls the shutdown server endpoint
func (c *Client) ShutdownServer(ctx context.Context) error {
	return c.RunCommand(ctx,
		curl.WithPath(ShutdownServerPath),
		curl.WithMethod(http.MethodPost))
}

// FailHealthCheck calls the endpoint to have the server start failing health checks
func (c *Client) FailHealthCheck(ctx context.Context) error {
	return c.RunCommand(ctx,
		curl.WithPath(fmt.Sprintf("%s/fail", HealthCheckPath)),
		curl.WithMethod(http.MethodPost))
}

// PassHealthCheck calls the endpoint to have the server start passing health checks
func (c *Client) PassHealthCheck(ctx context.Context) error {
	return c.RunCommand(ctx,
		curl.WithPath(fmt.Sprintf("%s/ok", HealthCheckPath)),
		curl.WithMethod(http.MethodPost))
}

// SetLogLevel calls the endpoint to change the log level for the server
func (c *Client) SetLogLevel(ctx context.Context, logLevel string) error {
	return c.RunCommand(ctx,
		curl.WithPath(LoggingPath),
		curl.WithQueryParameters(map[string]string{
			"level": logLevel,
		}),
		curl.WithMethod(http.MethodPost))
}

// GetServerInfo calls the endpoint to return the info for the server
func (c *Client) GetServerInfo(ctx context.Context) (*adminv3.ServerInfo, error) {
	var (
		serverInfo  adminv3.ServerInfo
		outLocation threadsafe.Buffer
	)

	err := c.ServerInfoCmd(ctx).WithStdout(&outLocation).Run().Cause()
	if err != nil {
		return nil, err
	}

	if err = protoutils.UnmarshalAllowUnknown(&outLocation, &serverInfo); err != nil {
		return nil, err
	}

	return &serverInfo, nil
}

// GetSingleListenerFromDynamicListeners queries for a single, active dynamic listener in the envoy config dump
// and returns it as an envoy v3.Listener. This helper will only work if the provided name_regex matches a single dynamic_listener
// but will always use the first set of configs returned regardless
func (c *Client) GetSingleListenerFromDynamicListeners(
	ctx context.Context,
	listenerNameRegex string,
) (*listenerv3.Listener, error) {
	queryParams := map[string]string{
		"resource":   "dynamic_listeners",
		"name_regex": listenerNameRegex,
	}
	cfgDump, err := c.GetConfigDump(ctx, queryParams)
	if err != nil {
		return nil, fmt.Errorf("could not get envoy config_dump from adminClient: %w", err)
	}

	configs := cfgDump.GetConfigs()

	// if no dynamic listeners name matching listenerNameRegex or
	// before envoy is full configured, dynamic listeners will be missing
	// and envoy will return an empty object json object `{}`
	if len(configs) == 0 {
		return nil, fmt.Errorf("could not get config: config is empty")
	}

	listenerDump := adminv3.ListenersConfigDump_DynamicListener{}
	err = configs[0].UnmarshalTo(&listenerDump)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal envoy config_dump: %w", err)
	}

	listener := listenerv3.Listener{}
	err = listenerDump.GetActiveState().GetListener().UnmarshalTo(&listener)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal listener from listener dump: %w", err)
	}
	return &listener, nil
}

// WriteEnvoyDumpToZip will dump config, stats, clusters and listeners to zipfile in the current directory.
// Useful for diagnostics or testing
func (c *Client) WriteEnvoyDumpToZip(ctx context.Context, options DumpOptions, zip *zip.Writer) error {
	configParams := make(map[string]string)
	if options.ConfigIncludeEDS {
		configParams["include_eds"] = "on"
	}

	// zip writer has the benefit of not requiring tmpdirs or file ops (all in mem)
	// - but it can't support async writes, so do these sequentally
	// Also don't join errors, we want to fast-fail
	if err := c.ServerInfoCmd(ctx).WithStdout(fileInArchive(zip, "server_info.json")).Run().Cause(); err != nil {
		return err
	}
	if err := c.ConfigDumpCmd(ctx, configParams).WithStdout(fileInArchive(zip, "config.json")).Run().Cause(); err != nil {
		return err
	}
	if err := c.StatsCmd(ctx, nil).WithStdout(fileInArchive(zip, "stats.txt")).Run().Cause(); err != nil {
		return err
	}
	if err := c.ClustersCmd(ctx).WithStdout(fileInArchive(zip, "clusters.txt")).Run().Cause(); err != nil {
		return err
	}
	if err := c.ListenersCmd(ctx).WithStdout(fileInArchive(zip, "listeners.txt")).Run().Cause(); err != nil {
		return err
	}

	return nil
}

// fileInArchive creates a file at the given path within the archive, and returns the file object for writing.
func fileInArchive(w *zip.Writer, path string) io.Writer {
	f, err := w.Create(path)
	if err != nil {
		fmt.Printf("unable to create file: %f\n", err)
	}
	return f
}
