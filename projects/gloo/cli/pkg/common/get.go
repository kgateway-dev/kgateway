package common

import (
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/avast/retry-go"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/gloo/pkg/cliutil"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	ratelimit "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/debug"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"google.golang.org/grpc"
)

func GetVirtualServices(name string, opts *options.Options) (v1.VirtualServiceList, error) {
	var virtualServiceList v1.VirtualServiceList
	virtualServiceClient := helpers.MustNamespacedVirtualServiceClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	if name == "" {
		virtualServices, err := virtualServiceClient.List(opts.Metadata.GetNamespace(),
			clients.ListOpts{Ctx: opts.Top.Ctx, Selector: opts.Get.Selector.MustMap()})
		if err != nil {
			return nil, err
		}
		virtualServiceList = append(virtualServiceList, virtualServices...)
	} else {
		virtualService, err := virtualServiceClient.Read(opts.Metadata.GetNamespace(), name, clients.ReadOpts{Ctx: opts.Top.Ctx})
		if err != nil {
			return nil, err
		}
		opts.Metadata.Name = name
		virtualServiceList = append(virtualServiceList, virtualService)
	}

	return virtualServiceList, nil
}

func GetRouteTables(name string, opts *options.Options) (v1.RouteTableList, error) {
	var routeTableList v1.RouteTableList

	routeTableClient := helpers.MustNamespacedRouteTableClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	if name == "" {
		routeTables, err := routeTableClient.List(opts.Metadata.GetNamespace(),
			clients.ListOpts{Ctx: opts.Top.Ctx, Selector: opts.Get.Selector.MustMap()})
		if err != nil {
			return nil, err
		}
		routeTableList = append(routeTableList, routeTables...)
	} else {
		routeTable, err := routeTableClient.Read(opts.Metadata.GetNamespace(), name, clients.ReadOpts{Ctx: opts.Top.Ctx})
		if err != nil {
			return nil, err
		}
		opts.Metadata.Name = name
		routeTableList = append(routeTableList, routeTable)
	}

	return routeTableList, nil
}

func GetUpstreams(name string, opts *options.Options) (gloov1.UpstreamList, error) {
	var list gloov1.UpstreamList

	usClient := helpers.MustNamespacedUpstreamClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	if name == "" {
		uss, err := usClient.List(opts.Metadata.GetNamespace(),
			clients.ListOpts{Ctx: opts.Top.Ctx, Selector: opts.Get.Selector.MustMap()})
		if err != nil {
			return nil, err
		}
		list = append(list, uss...)
	} else {
		us, err := usClient.Read(opts.Metadata.GetNamespace(), name, clients.ReadOpts{Ctx: opts.Top.Ctx})
		if err != nil {
			return nil, err
		}
		opts.Metadata.Name = name
		list = append(list, us)
	}

	return list, nil
}

func GetUpstreamGroups(name string, opts *options.Options) (gloov1.UpstreamGroupList, error) {
	var list gloov1.UpstreamGroupList

	ugsClient := helpers.MustNamespacedUpstreamGroupClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	if name == "" {
		ugs, err := ugsClient.List(opts.Metadata.GetNamespace(),
			clients.ListOpts{Ctx: opts.Top.Ctx, Selector: opts.Get.Selector.MustMap()})
		if err != nil {
			return nil, err
		}
		list = append(list, ugs...)
	} else {
		ugs, err := ugsClient.Read(opts.Metadata.GetNamespace(), name, clients.ReadOpts{Ctx: opts.Top.Ctx})
		if err != nil {
			return nil, err
		}
		opts.Metadata.Name = name
		list = append(list, ugs)
	}

	return list, nil
}

func GetSettings(opts *options.Options) (*gloov1.Settings, error) {
	client, err := helpers.SettingsClient(opts.Top.Ctx, []string{opts.Metadata.GetNamespace()})
	if err != nil {
		return nil, err
	}
	return client.Read(opts.Metadata.GetNamespace(), defaults.SettingsName, clients.ReadOpts{Ctx: opts.Top.Ctx})
}

func GetProxies(name string, opts *options.Options) (gloov1.ProxyList, error) {
	settings, err := GetSettings(opts)
	if err != nil {
		return nil, err
	}

	proxyEndpointPort, err := computeProxyEndpointPort(settings)
	if err != nil {
		return nil, err
	}
	return getProxiesFromGrpc(name, opts.Metadata.GetNamespace(), opts, proxyEndpointPort)
}

// ListProxiesFromSettings retrieves proxies from the proxy debug endpoint, or from kubernetes if the proxy debug endpoint is not available
// Takes in a settings object to determine whether the proxy debug endpoint is available
func ListProxiesFromSettings(namespace string, opts *options.Options, settings *gloov1.Settings) (gloov1.ProxyList, error) {
	proxyEndpointPort, err := computeProxyEndpointPort(settings)
	if err != nil {
		return nil, err
	}

	return getProxiesFromGrpc("", namespace, opts, proxyEndpointPort)
}

func computeProxyEndpointPort(settings *gloov1.Settings) (string, error) {
	proxyEndpointAddress := settings.GetGloo().GetProxyDebugBindAddr()
	_, proxyEndpointPort, err := net.SplitHostPort(proxyEndpointAddress)
	return proxyEndpointPort, err
}

// Used to retrieve proxies from the proxy debug endpoint in newer versions of gloo
// if name is empty, return all proxies
func getProxiesFromGrpc(name string, namespace string, opts *options.Options, proxyEndpointPort string) (gloov1.ProxyList, error) {
	remotePort, err := strconv.Atoi(proxyEndpointPort)
	if err != nil {
		return nil, err
	}

	return GetProxiesFromControlPlane(
		opts,
		&debug.ProxyEndpointRequest{
			Name:      name,
			Namespace: namespace,
			Source:    "",  // this method does not support the source API
			Selector:  nil, // this method does not support the selector API
		},
		remotePort)
}

// GetProxiesFromControlPlane executes a gRPC request against the Control Plane (Gloo) a a given port (proxyEndpointPort).
// Proxies are an intermediate resource that are often persisted in-memory in the Control Plane.
// To improve debuggability, we expose an API to return the current proxies, and rely on this CLI method to expose that to users
func GetProxiesFromControlPlane(opts *options.Options, proxyRequest *debug.ProxyEndpointRequest, proxyEndpointPort int) (gloov1.ProxyList, error) {
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
		kubeutils.WithRemotePort(proxyEndpointPort),
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
		r, err := pxClient.GetProxies(opts.Top.Ctx, proxyRequest,
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

func GetAuthConfigs(name string, opts *options.Options) (extauthv1.AuthConfigList, error) {
	var authConfigList extauthv1.AuthConfigList

	authConfigClient := helpers.MustNamespacedAuthConfigClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	if name == "" {
		authConfigs, err := authConfigClient.List(opts.Metadata.GetNamespace(),
			clients.ListOpts{Ctx: opts.Top.Ctx, Selector: opts.Get.Selector.MustMap()})
		if err != nil {
			return nil, err
		}
		authConfigList = append(authConfigList, authConfigs...)
	} else {
		authConfig, err := authConfigClient.Read(opts.Metadata.GetNamespace(), name, clients.ReadOpts{Ctx: opts.Top.Ctx})
		if err != nil {
			return nil, err
		}
		opts.Metadata.Name = name
		authConfigList = append(authConfigList, authConfig)
	}

	return authConfigList, nil
}

func GetRateLimitConfigs(name string, opts *options.Options) (ratelimit.RateLimitConfigList, error) {
	var ratelimitConfigList ratelimit.RateLimitConfigList

	ratelimitConfigClient := helpers.MustNamespacedRateLimitConfigClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	if name == "" {
		ratelimitConfigs, err := ratelimitConfigClient.List(opts.Metadata.GetNamespace(),
			clients.ListOpts{Ctx: opts.Top.Ctx, Selector: opts.Get.Selector.MustMap()})
		if err != nil {
			return nil, err
		}
		ratelimitConfigList = append(ratelimitConfigList, ratelimitConfigs...)
	} else {
		ratelimitConfig, err := ratelimitConfigClient.Read(opts.Metadata.GetNamespace(), name, clients.ReadOpts{Ctx: opts.Top.Ctx})
		if err != nil {
			return nil, err
		}
		opts.Metadata.Name = name
		ratelimitConfigList = append(ratelimitConfigList, ratelimitConfig)
	}

	return ratelimitConfigList, nil
}

func GetName(args []string, opts *options.Options) string {
	if len(args) > 0 {
		return args[0]
	}
	return opts.Metadata.GetName()
}
