package krtcollections

import (
	"context"

	istiogvr "istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"

	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func InitCollectionsWithGateways(ctx context.Context,
	isOurGw func(gw *gwv1.Gateway) bool,
	kubeRawGateways krt.Collection[*gwv1.Gateway],
	httpRoutes krt.Collection[*gwv1.HTTPRoute],
	tcproutes krt.Collection[*gwv1a2.TCPRoute],
	refgrants *RefGrantIndex,
	extensions extensionsplug.Plugin, krtopts krtutil.KrtOptions) (*GatewayIndex, *RoutesIndex, krt.Collection[ir.Upstream], krt.Collection[ir.EndpointsForUpstream]) {

	policies := NewPolicyIndex(krtopts, extensions.ContributesPolicies)

	var backendRefPlugins []extensionsplug.GetBackendForRefPlugin
	for _, ext := range extensions.ContributesPolicies {
		if ext.GetBackendForRef != nil {
			backendRefPlugins = append(backendRefPlugins, ext.GetBackendForRef)
		}
	}

	upstreamIndex := NewUpstreamIndex(krtopts, backendRefPlugins, policies)
	finalUpstreams, endpointIRs := initUpstreams(ctx, extensions, upstreamIndex, krtopts)

	kubeGateways := NewGatewayIndex(krtopts, isOurGw, policies, kubeRawGateways)

	routes := NewRoutesIndex(krtopts, httpRoutes, tcproutes, policies, upstreamIndex, refgrants)
	return kubeGateways, routes, finalUpstreams, endpointIRs
}

func InitCollections(ctx context.Context,
	extensions extensionsplug.Plugin,
	istioClient kube.Client,
	isOurGw func(gw *gwv1.Gateway) bool,
	refgrants *RefGrantIndex,
	krtopts krtutil.KrtOptions) (*GatewayIndex, *RoutesIndex, krt.Collection[ir.Upstream], krt.Collection[ir.EndpointsForUpstream]) {

	kubeRawGateways := krtutil.SetupCollectionDynamic[gwv1.Gateway](
		ctx,
		istioClient,
		istiogvr.KubernetesGateway_v1,
		krtopts.ToOptions("KubeGateways")...,
	)
	httpRoutes := krtutil.SetupCollectionDynamic[gwv1.HTTPRoute](
		ctx,
		istioClient,
		istiogvr.HTTPRoute_v1,
		krtopts.ToOptions("HTTPRoute")...,
	)

	tcproutes := krtutil.SetupCollectionDynamic[gwv1a2.TCPRoute](
		ctx,
		istioClient,
		istiogvr.TCPRoute,
		krtopts.ToOptions("TCPRoute")...,
	)

	return InitCollectionsWithGateways(ctx, isOurGw, kubeRawGateways, httpRoutes, tcproutes, refgrants, extensions, krtopts)
}

func initUpstreams(ctx context.Context,
	extensions extensionsplug.Plugin, upstreamIndex *UpstreamIndex, krtopts krtutil.KrtOptions) (krt.Collection[ir.Upstream], krt.Collection[ir.EndpointsForUpstream]) {

	allEndpoints := []krt.Collection[ir.EndpointsForUpstream]{}
	for k, col := range extensions.ContributesUpstreams {
		if col.Upstreams != nil {
			upstreamIndex.AddUpstreams(k, col.Upstreams)
		}
		if col.Endpoints != nil {
			allEndpoints = append(allEndpoints, col.Endpoints)
		}
	}

	finalUpstreams := krt.JoinCollection(upstreamIndex.Upstreams(), krtopts.ToOptions("FinalUpstreams")...)

	// build Endpoint intermediate representation from kubernetes service and extensions
	// TODO move kube service to be an extension
	endpointIRs := krt.JoinCollection(allEndpoints, krtopts.ToOptions("EndpointIRs")...)
	return finalUpstreams, endpointIRs
}
