package common

import (
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	ratelimit "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
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

func GetProxies(name string, opts *options.Options) (gloov1.ProxyList, error) {
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
