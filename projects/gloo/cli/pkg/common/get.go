package common

import (
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

func GetVirtualServices(name string, opts *options.Options) (v1.VirtualServiceList, error) {
	var virtualServiceList v1.VirtualServiceList

	virtualServiceClient := helpers.MustVirtualServiceClient()
	if name == "" {
		virtualServices, err := virtualServiceClient.List(opts.Metadata.Namespace,
			clients.ListOpts{Ctx: opts.Top.Ctx, Selector: opts.Get.Selector.MustMap()})
		if err != nil {
			return nil, err
		}
		virtualServiceList = append(virtualServiceList, virtualServices...)
	} else {
		virtualService, err := virtualServiceClient.Read(opts.Metadata.Namespace, name, clients.ReadOpts{Ctx: opts.Top.Ctx})
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

	routeTableClient := helpers.MustRouteTableClient()
	if name == "" {
		routeTables, err := routeTableClient.List(opts.Metadata.Namespace,
			clients.ListOpts{Ctx: opts.Top.Ctx, Selector: opts.Get.Selector.MustMap()})
		if err != nil {
			return nil, err
		}
		routeTableList = append(routeTableList, routeTables...)
	} else {
		routeTable, err := routeTableClient.Read(opts.Metadata.Namespace, name, clients.ReadOpts{Ctx: opts.Top.Ctx})
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

	usClient := helpers.MustUpstreamClient()
	if name == "" {
		uss, err := usClient.List(opts.Metadata.Namespace,
			clients.ListOpts{Ctx: opts.Top.Ctx, Selector: opts.Get.Selector.MustMap()})
		if err != nil {
			return nil, err
		}
		list = append(list, uss...)
	} else {
		us, err := usClient.Read(opts.Metadata.Namespace, name, clients.ReadOpts{Ctx: opts.Top.Ctx})
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

	ugsClient := helpers.MustUpstreamGroupClient()
	if name == "" {
		ugs, err := ugsClient.List(opts.Metadata.Namespace,
			clients.ListOpts{Ctx: opts.Top.Ctx, Selector: opts.Get.Selector.MustMap()})
		if err != nil {
			return nil, err
		}
		list = append(list, ugs...)
	} else {
		ugs, err := ugsClient.Read(opts.Metadata.Namespace, name, clients.ReadOpts{Ctx: opts.Top.Ctx})
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

	pxClient := helpers.MustProxyClient()
	if name == "" {
		uss, err := pxClient.List(opts.Metadata.Namespace,
			clients.ListOpts{Ctx: opts.Top.Ctx, Selector: opts.Get.Selector.MustMap()})
		if err != nil {
			return nil, err
		}
		list = append(list, uss...)
	} else {
		us, err := pxClient.Read(opts.Metadata.Namespace, name, clients.ReadOpts{Ctx: opts.Top.Ctx})
		if err != nil {
			return nil, err
		}
		opts.Metadata.Name = name
		list = append(list, us)
	}

	return list, nil
}

func GetName(args []string, opts *options.Options) string {
	if len(args) > 0 {
		return args[0]
	}
	return opts.Metadata.Name
}
