package istio_integration

import (
	"context"
	"fmt"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var (
	_ plugins.RoutePlugin = new(plugin)
)

const (
	ExtensionName = "istio_integration"
)

// Handles transformations required to integrate with Istio
type plugin struct {
}

func NewPlugin(ctx context.Context) *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
}

// When istio integration is enabled, we need to access k8s services using a host that istio will recognize (servicename.namespace)
// We do this by adding a hostRewrite for kube destinations kube upstreams. In case the upstream also wants the original host,
// we also set x-forwarded-host
// We ignore other destinations and routes that already have a rewrite applied and return an error if we can't look up an Upstream.
func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	//check if this is a kubernetes destination
	dest, ok := in.GetRouteAction().GetDestination().(*v1.RouteAction_Single)
	if !ok {
		// With current use cases it only makes sense to handle k8s single destinations
		// If this use case expands we would need to figure out Istio's expected hostname format for each type of destination.
		return nil
	}
	hostInMesh, err := GetHostFromDestination(dest, params.Snapshot.Upstreams)
	if err != nil {
		return err
	}
	if hostInMesh == "" {
		return nil
	}
	routeAction, ok := out.GetAction().(*envoy_config_route_v3.Route_Route)
	if !ok || routeAction.Route.GetHostRewriteSpecifier() != nil {
		// This only applies to routes that do not already have a host rewrite
		return nil
	}
	//Set the host rewrite and x-forwarded-host header
	routeAction.Route.HostRewriteSpecifier = &envoy_config_route_v3.RouteAction_HostRewriteLiteral{
		HostRewriteLiteral: hostInMesh,
	}
	routeAction.Route.AppendXForwardedHost = true
	return nil
}

// Take the RouteAction_single and find the kubernetes service name and namespace
// Return the hostname to rewrite: serviceName.namespace if the destination is a kubernetes upstream or kube destination
// Return an empty string for another type of destination
// Return an error if the destination is a gloo upstream and we cannot look it up
func GetHostFromDestination(dest *v1.RouteAction_Single, upstreams v1.UpstreamList) (string, error) {
	var name, namespace string
	if single, ok := dest.Single.GetDestinationType().(*v1.Destination_Upstream); ok {
		us, err := upstreams.Find(single.Upstream.GetNamespace(), single.Upstream.GetName())
		if err != nil {
			return "", err
		}
		kubeUs, ok := us.GetUpstreamType().(*v1.Upstream_Kube)
		if !ok {
			// kube upstreams are the only things that we need to access within the mesh
			return "", nil
		}
		name = kubeUs.Kube.GetServiceName()
		namespace = kubeUs.Kube.GetServiceNamespace()
	} else if d, ok := dest.Single.GetDestinationType().(*v1.Destination_Kube); ok {
		name = d.Kube.GetRef().GetName()
		namespace = d.Kube.GetRef().GetNamespace()
	}
	// Any unhandled destination type
	if name == "" || namespace == "" {
		return "", nil
	}
	return fmt.Sprintf("%s.%s", name, namespace), nil
}
