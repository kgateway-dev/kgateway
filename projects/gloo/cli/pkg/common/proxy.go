package common

import (
	"context"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/portforward"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/debug"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/errors"
	"google.golang.org/grpc"
)

// GetProxies retrieves the proxies from the Control Plane via the ProxyEndpointServer API
// This is utilized by `glooctl get proxy` to return the content of Proxies
func GetProxies(name string, opts *options.Options) (gloov1.ProxyList, error) {
	settings, err := GetSettings(opts)
	if err != nil {
		return nil, err
	}

	proxyEndpointPort, err := computeProxyEndpointPort(settings)
	if err != nil {
		return nil, err
	}
	// get the namespace where Proxies are written. if discovery namespace is empty, defaults to the gloo install namespace
	writeNamespace := settings.GetDiscoveryNamespace()
	if writeNamespace == "" {
		writeNamespace = opts.Metadata.GetNamespace()
	}

	return getProxiesFromControlPlane(opts, name, proxyEndpointPort, writeNamespace)
}

// ListProxiesFromSettings retrieves the proxies from the Control Plane via the ProxyEndpointServer API
// This is utilized by `glooctl check` to report the statuses of Proxies
func ListProxiesFromSettings(namespace string, opts *options.Options, settings *gloov1.Settings) (gloov1.ProxyList, error) {
	proxyEndpointPort, err := computeProxyEndpointPort(settings)
	if err != nil {
		return nil, err
	}

	return listProxiesFromControlPlane(opts, namespace, proxyEndpointPort)
}

func computeProxyEndpointPort(settings *gloov1.Settings) (string, error) {
	proxyEndpointAddress := settings.GetGloo().GetProxyDebugBindAddr()
	if proxyEndpointAddress == "" {
		// This can occur if you are querying a Settings object that was created before the ProxyDebugBindAddr API
		// was introduced to the Settings CR. In practice, this should never occur, as the API has existed for many releases.
		return "", errors.Errorf("ProxyDebugBindAddr is empty. Consider upgrading the version of Gloo")
	}

	_, proxyEndpointPort, err := net.SplitHostPort(proxyEndpointAddress)
	return proxyEndpointPort, err
}

// this is only called by GetProxies which is called by `glooctl get proxy`
func getProxiesFromControlPlane(opts *options.Options, name string, proxyEndpointPort string, writeNamespace string) (gloov1.ProxyList, error) {
	proxyRequest := &debug.ProxyEndpointRequest{
		Name: name,
		// It is important that we use the writeNamespace (aka discoveryNamespace from Settings CR) here, as opposed to the opts.Metadata.Namespace.
		// The former is where Proxies will be searched, the latter is where Gloo is installed
		Namespace:          writeNamespace,
		ExpressionSelector: getSelectorFromOpts(opts.Get.Proxy),
	}

	return requestProxiesFromControlPlane(opts, proxyRequest, proxyEndpointPort)
}

// this is only called by ListProxiesFromSettings which is called by `glooctl check`
func listProxiesFromControlPlane(opts *options.Options, namespace, proxyEndpointPort string) (gloov1.ProxyList, error) {
	proxyRequest := &debug.ProxyEndpointRequest{
		Name:      "",
		Namespace: namespace,
		Selector:  opts.Get.Selector.MustMap(), // note: we don't currently provide a way for the user to set this selector
	}

	return requestProxiesFromControlPlane(opts, proxyRequest, proxyEndpointPort)
}

// getSelectorFromOpts returns a set-based selector based on the filtering specified by the GetProxy options.
// If no filtering is specified, the selector will be empty.
func getSelectorFromOpts(opts options.GetProxy) string {
	var translators []string
	if opts.EdgeGatewaySource {
		translators = append(translators, utils.GlooEdgeProxyValue)
	}
	if opts.K8sGatewaySource {
		translators = append(translators, utils.GatewayApiProxyValue)
	}
	if len(translators) > 0 {
		return utils.GetTranslatorSelectorExpression(translators...)
	}
	return ""
}

// requestProxiesFromControlPlane executes a gRPC request against the Control Plane (Gloo) against a given port (proxyEndpointPort).
// Proxies are an intermediate resource that are often persisted in-memory in the Control Plane.
// To improve debuggability, we expose an API to return the current proxies, and rely on this CLI method to expose that to users
func requestProxiesFromControlPlane(opts *options.Options, request *debug.ProxyEndpointRequest, proxyEndpointPort string) (gloov1.ProxyList, error) {
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

	requestCtx, cancel := context.WithTimeout(opts.Top.Ctx, 30*time.Second)
	defer cancel()

	portForwarder := portforward.NewPortForwarder(
		portforward.WithDeployment(kubeutils.GlooDeploymentName, opts.Metadata.GetNamespace()),
		portforward.WithRemotePort(remotePort),
		portforward.WithWriters(outWriter, errWriter),
	)
	if err := portForwarder.Start(
		requestCtx,
		retry.LastErrorOnly(true),
		retry.Delay(100*time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
		retry.Attempts(5),
	); err != nil {
		return nil, err
	}
	defer func() {
		portForwarder.Close()
		portForwarder.WaitForStop()
	}()

	var proxyEndpointResponse *debug.ProxyEndpointResponse
	requestErr := retry.Do(func() error {
		cc, err := grpc.DialContext(requestCtx, portForwarder.Address(), grpc.WithInsecure())
		if err != nil {
			return err
		}
		pxClient := debug.NewProxyEndpointServiceClient(cc)
		r, err := pxClient.GetProxies(requestCtx, request,
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
