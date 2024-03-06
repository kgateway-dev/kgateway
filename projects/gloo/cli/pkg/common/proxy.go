package common

import (
	debug2 "github.com/solo-io/gloo/projects/gloo/pkg/debug"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/avast/retry-go"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"

	"github.com/solo-io/gloo/pkg/cliutil"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/debug"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"google.golang.org/grpc"
)

func GetProxies(name string, opts *options.Options) (gloov1.ProxyList, error) {
	settings, err := GetSettings(opts)
	if err != nil {
		return nil, err
	}

	proxyEndpointPort, err := computeProxyEndpointPort(settings)
	if err != nil {
		return nil, err
	}

	return getProxiesFromControlPlane(
		opts,
		name,
		proxyEndpointPort)
}

// ListProxiesFromSettings retrieves proxies from the proxy debug endpoint, or from kubernetes if the proxy debug endpoint is not available
// Takes in a settings object to determine whether the proxy debug endpoint is available
func ListProxiesFromSettings(namespace string, opts *options.Options, settings *gloov1.Settings) (gloov1.ProxyList, error) {
	proxyEndpointPort, err := computeProxyEndpointPort(settings)
	if err != nil {
		return nil, err
	}

	return listProxiesFromControlPlane(opts, namespace, proxyEndpointPort)
}

func computeProxyEndpointPort(settings *gloov1.Settings) (string, error) {
	proxyEndpointAddress := settings.GetGloo().GetProxyDebugBindAddr()
	_, proxyEndpointPort, err := net.SplitHostPort(proxyEndpointAddress)
	return proxyEndpointPort, err
}

func getProxiesFromControlPlane(opts *options.Options, name string, proxyEndpointPort string) (gloov1.ProxyList, error) {
	proxyRequest := &debug.ProxyEndpointRequest{
		Name: name,
		// It is important that we use the Proxy.Namespace here, as opposed to the opts.Metadata.Namespace
		// The former is where Proxies will be searched, the latter is where Gloo is installed
		Namespace: opts.Get.Proxy.Namespace,
		Source:    getProxySource(opts.Get.Proxy),
		Selector:  opts.Get.Selector.MustMap(),
	}

	return RequestProxiesFromControlPlane(opts, proxyRequest, proxyEndpointPort)
}

func listProxiesFromControlPlane(opts *options.Options, namespace, proxyEndpointPort string) (gloov1.ProxyList, error) {
	proxyRequest := &debug.ProxyEndpointRequest{
		Name:      "",
		Namespace: namespace,
		Source:    getProxySource(opts.Get.Proxy),
		Selector:  opts.Get.Selector.MustMap(),
	}

	return RequestProxiesFromControlPlane(opts, proxyRequest, proxyEndpointPort)
}

func getProxySource(proxy options.GetProxy) string {
	proxySource := "" // empty string is considered "all"
	if proxy.EdgeGatewaySource && !proxy.K8sGatewaySource {
		proxySource = debug2.EdgeGatewaySourceName
	}
	if !proxy.EdgeGatewaySource && proxy.K8sGatewaySource {
		proxySource = debug2.K8sGatewaySourceName
	}
	return proxySource
}

// RequestProxiesFromControlPlane executes a gRPC request against the Control Plane (Gloo) a a given port (proxyEndpointPort).
// Proxies are an intermediate resource that are often persisted in-memory in the Control Plane.
// To improve debuggability, we expose an API to return the current proxies, and rely on this CLI method to expose that to users
func RequestProxiesFromControlPlane(opts *options.Options, request *debug.ProxyEndpointRequest, proxyEndpointPort string) (gloov1.ProxyList, error) {
	remotePort, err := strconv.Atoi(proxyEndpointPort)
	if err != nil {
		return nil, err
	}

	logger := cliutil.GetLogger()
	var outWriter, errWriter io.Writer
	errWriter = io.MultiWriter(logger, os.Stderr)
	if opts.Top.Verbose {
		outWriter = io.MultiWriter(logger, os.Stdout)
	} else {
		outWriter = logger
	}

	portForwarder := kubeutils.NewPortForwarder(
		kubeutils.WithDeployment(kubeutils.GlooDeploymentName, opts.Metadata.GetNamespace()),
		kubeutils.WithRemotePort(remotePort),
		kubeutils.WithWriters(outWriter, errWriter),
	)
	if err := portForwarder.Start(
		opts.Top.Ctx,
		retry.LastErrorOnly(true),
		retry.Delay(100*time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
		retry.Attempts(5),
	); err != nil {
		return nil, err
	}
	defer portForwarder.Close()

	var proxyEndpointResponse *debug.ProxyEndpointResponse
	requestErr := retry.Do(func() error {
		cc, err := grpc.DialContext(opts.Top.Ctx, portForwarder.Address(), grpc.WithInsecure())
		if err != nil {
			return err
		}
		pxClient := debug.NewProxyEndpointServiceClient(cc)
		r, err := pxClient.GetProxies(opts.Top.Ctx, request,
			// Some proxies can become very large and exceed the default 100Mb limit
			// For this reason we want remove the limit but will settle for a limit of MaxInt32
			// as we don't anticipate proxies to exceed this
			grpc.MaxCallRecvMsgSize(math.MaxInt32),
		)
		proxyEndpointResponse = r
		return err
	},
		retry.LastErrorOnly(true),
		retry.Delay(100*time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
		retry.Attempts(5),
	)

	if requestErr != nil {
		return nil, requestErr
	}

	return proxyEndpointResponse.GetProxies(), nil
}
