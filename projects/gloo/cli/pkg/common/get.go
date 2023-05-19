package common

import (
	"context"
	"math"
	"net"
	"strconv"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/hashicorp/go-multierror"
	errors "github.com/rotisserie/eris"
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
	proxyEndpointPort := computeProxyEndpointPort(opts.Top.Ctx, settings)
	if proxyEndpointPort != "" {
		return getProxiesFromGrpc(name, opts.Metadata.GetNamespace(), opts, proxyEndpointPort)
	}
	return getProxiesFromK8s(name, opts)
}

// ListProxiesFromSettings retrieves proxies from the proxy debug endpoint, or from kubernetes if the proxy debug endpoint is not available
// Takes in a settings object to determine whether the proxy debug endpoint is available
func ListProxiesFromSettings(namespace string, opts *options.Options, settings *gloov1.Settings) (gloov1.ProxyList, error) {
	proxyEndpointPort := computeProxyEndpointPort(opts.Top.Ctx, settings)
	if proxyEndpointPort != "" {
		return getProxiesFromGrpc("", namespace, opts, proxyEndpointPort)
	}
	return getProxiesFromK8s("", opts)
}

func computeProxyEndpointPort(ctx context.Context, settings *gloov1.Settings) string {

	proxyEndpointAddress := settings.GetGloo().GetProxyDebugBindAddr()
	_, proxyEndpointPort, err := net.SplitHostPort(proxyEndpointAddress)
	if err != nil {
		proxyEndpointPort = ""
		contextutils.LoggerFrom(ctx).Debugf("Could not parse the port for the proxy debug endpoint. " +
			"Will check for proxies persisted to etcd.")
	}
	return proxyEndpointPort
}

// This is necessary for older versions of gloo
// if name is empty, return all proxies
func getProxiesFromK8s(name string, opts *options.Options) (gloov1.ProxyList, error) {
	var list gloov1.ProxyList
	pxClient := helpers.MustNamespacedProxyClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	if name == "" {
		uss, err := pxClient.List(opts.Metadata.GetNamespace(),
			clients.ListOpts{Ctx: opts.Top.Ctx, Selector: opts.Get.Selector.MustMap()})
		if err != nil {
			return nil, err
		}
		list = append(list, uss...)
	} else {
		us, err := pxClient.Read(opts.Metadata.GetNamespace(), name, clients.ReadOpts{Ctx: opts.Top.Ctx})
		if err != nil {
			return nil, err
		}
		opts.Metadata.Name = name
		list = append(list, us)
	}

	return list, nil
}

// Used to retrieve proxies from the proxy debug endpoint in newer versions of gloo
// if name is empty, return all proxies
func getProxiesFromGrpc(name string, namespace string, opts *options.Options, proxyEndpointPort string) (gloov1.ProxyList, error) {

	options := []grpc.CallOption{
		// Some proxies can become very large and exceed the default 100Mb limit
		// For this reason we want remove the limit but will settle for a limit of MaxInt32
		// as we don't anticipate proxies to exceed this
		grpc.MaxCallRecvMsgSize(int(math.MaxInt32)),
	}

	freePort, err := cliutil.GetFreePort()
	if err != nil {
		return nil, err
	}
	localPort := strconv.Itoa(freePort)
	portFwdCmd, err := cliutil.PortForward(opts.Metadata.GetNamespace(), "deployment/gloo",
		localPort, proxyEndpointPort, opts.Top.Verbose)
	if portFwdCmd.Process != nil {
		defer portFwdCmd.Process.Release()
		defer portFwdCmd.Process.Kill()
	}
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	localCtx, cancel := context.WithTimeout(opts.Top.Ctx, time.Second*30)
	defer cancel()
	// wait for port-forward to be ready
	retryInterval := time.Millisecond * 250
	errs := make(chan error)
	resp := make(chan *debug.ProxyEndpointResponse)
	go func() {
		for {
			select {
			case <-localCtx.Done():
				return
			default:
			}
			cc, err := grpc.DialContext(localCtx, "localhost:"+localPort, grpc.WithInsecure())
			if err != nil {
				errs <- err
				time.Sleep(retryInterval)
				continue
			}
			pxClient := debug.NewProxyEndpointServiceClient(cc)
			r, err := pxClient.GetProxies(opts.Top.Ctx, &debug.ProxyEndpointRequest{
				Name:      name,
				Namespace: namespace,
			}, options...)
			if err != nil {
				errs <- err
				time.Sleep(retryInterval)
				continue
			}
			resp <- r
		}
	}()

	var multiErr *multierror.Error
	for {
		select {
		case err := <-errs:
			multiErr = multierror.Append(multiErr, err)
		case r := <-resp:
			return r.GetProxies(), nil
		case <-localCtx.Done():
			return nil, errors.Errorf("timed out trying to connect to localhost during port-forward, errors: %v", multiErr)
		}
	}

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
