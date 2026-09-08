package collections

import (
	"context"

	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	"istio.io/istio/pkg/util/smallset"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	kmetrics "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func (c *CommonCollections) InitCollections(
	ctx context.Context,
	controllerNames smallset.Set[string],
	plugins pluginsdk.Plugin,
	globalSettings apisettings.Settings,
) (*krtcollections.GatewayIndex, *krtcollections.RoutesIndex, *krtcollections.BackendIndex, krt.Collection[ir.EndpointsForBackend]) {
	apiclient.RegisterTypes()

	// discovery filter
	filter := kclient.Filter{ObjectFilter: c.Client.ObjectFilter()}

	//nolint:forbidigo // ObjectFilter is not needed for this client as it is cluster scoped
	gatewayClasses := krt.WrapClient(kclient.New[*gwv1.GatewayClass](c.Client), c.KrtOpts.ToOptions("KubeGatewayClasses")...)

	namespaces, _ := krtcollections.NewNamespaceCollection(ctx, c.Client, c.KrtOpts)

	kubeRawGateways := krt.WrapClient(kclient.NewFilteredDelayed[*gwv1.Gateway](c.Client, wellknown.GatewayGVR, filter), c.KrtOpts.ToOptions("KubeGateways")...)
	metrics.RegisterEvents(kubeRawGateways, kmetrics.GetResourceMetricEventHandler[*gwv1.Gateway]())

	var kubeRawListenerSets krt.Collection[*gwv1.ListenerSet]
	promotedListenerSets := krt.WrapClient(
		kclient.NewDelayedInformer[*gwv1.ListenerSet](c.Client, wellknown.ListenerSetGVR, kubetypes.StandardInformer, filter),
		c.KrtOpts.ToOptions("KubePromotedListenerSets")...,
	)
	// ON_EXPERIMENTAL_PROMOTION : Remove this block
	// Ref: https://github.com/kgateway-dev/kgateway/issues/12827
	if globalSettings.EnableExperimentalGatewayAPIFeatures {
		legacyListenerSetsRaw := krt.WrapClient(
			newDelayedDynamicUnstructuredInformer(ctx, c.Client, wellknown.XListenerSetGVR, filter),
			c.KrtOpts.ToOptions("KubeLegacyXListenerSets")...,
		)
		legacyListenerSets := krt.NewManyCollection(legacyListenerSetsRaw, func(kctx krt.HandlerContext, in *unstructured.Unstructured) []*gwv1.ListenerSet {
			if ls := convertLegacyXListenerSetToV1(in); ls != nil {
				return []*gwv1.ListenerSet{ls}
			}
			return nil
		}, c.KrtOpts.ToOptions("KubeLegacyXListenerSetsConverted")...)
		kubeRawListenerSets = krt.JoinCollection(
			[]krt.Collection[*gwv1.ListenerSet]{promotedListenerSets, legacyListenerSets},
			c.KrtOpts.ToOptions("KubeListenerSets")...,
		)
	} else {
		kubeRawListenerSets = promotedListenerSets
	}
	metrics.RegisterEvents(kubeRawListenerSets, kmetrics.GetResourceMetricEventHandler[*gwv1.ListenerSet]())

	c.RawGateways = kubeRawGateways
	c.RawListenerSets = kubeRawListenerSets

	policies := krtcollections.NewPolicyIndex(c.KrtOpts, plugins.ContributesPolicies, globalSettings)
	for _, plugin := range plugins.ContributesPolicies {
		if plugin.Policies != nil {
			metrics.RegisterEvents(plugin.Policies, kmetrics.GetResourceMetricEventHandler[ir.PolicyWrapper]())
		}
	}

	gateways := krtcollections.NewGatewayIndex(krtcollections.GatewayIndexConfig{
		KrtOpts:             c.KrtOpts,
		ControllerNames:     controllerNames,
		EnvoyControllerName: c.ControllerName,
		PolicyIndex:         policies,
		Gateways:            kubeRawGateways,
		ListenerSets:        kubeRawListenerSets,
		GatewayClasses:      gatewayClasses,
		Namespaces:          namespaces,
	},
		krtcollections.WithGatewayForDeployerTransformationFunc(c.options.gatewayForDeployerTransformationFunc),
		krtcollections.WithGatewayForEnvoyTransformationFunc(c.options.gatewayForEnvoyTransformationFunc),
	)

	// create the KRT clients, remember to also register any needed types in the type registration setup.
	httpRoutes := krt.WrapClient(kclient.NewFilteredDelayed[*gwv1.HTTPRoute](c.Client, wellknown.HTTPRouteGVR, filter), c.KrtOpts.ToOptions("HTTPRoute")...)
	metrics.RegisterEvents(httpRoutes, kmetrics.GetResourceMetricEventHandler[*gwv1.HTTPRoute]())

	// Resolve which TCPRoute API versions to watch and to write status through. The same
	// list drives both, so a version we watch is always one we can write and vice versa.
	// TCPRoute is standard as of Gateway API v1.6; pre-v1 versions stay behind the
	// experimental feature flag for compatibility with older Gateway API channels.
	versionSource := newRouteVersionSource(c.Client)
	includeLegacyRouteVersions := globalSettings.EnableExperimentalGatewayAPIFeatures
	tcpRouteWriteGVRs := selectRouteGVRs(
		resolveRouteVersions(ctx, versionSource, wellknown.TCPRouteCRDName, tcpRouteGVRs),
		tcpRouteGVRs,
		includeLegacyRouteVersions,
	)
	// Each candidate's informer is gated on that candidate being the served version we
	// prefer, re-read at gate time rather than reused from the startup resolution above.
	// When the resolution was authoritative the gate just confirms it; when it was not, this
	// is what keeps an unserved candidate from starting a watch that could only 404.
	tcpRouteGate := func(gvr schema.GroupVersionResource) informerGate {
		return routeInformerGate(versionSource, wellknown.TCPRouteCRDName, tcpRouteGVRs, includeLegacyRouteVersions, gvr)
	}
	tcpRouteCollections := make([]krt.Collection[*gwv1a2.TCPRoute], 0, len(tcpRouteWriteGVRs))
	for _, tcpRouteGVR := range tcpRouteWriteGVRs {
		switch tcpRouteGVR.Version {
		case gwv1.GroupVersion.Version:
			tcpRoutesV1 := krt.WrapClient(
				newGatedTypedInformer(ctx, tcpRouteGVR, tcpRouteGate(tcpRouteGVR), func() kclient.Informer[*gwv1.TCPRoute] {
					return kclient.NewFiltered[*gwv1.TCPRoute](c.Client, filter)
				}),
				c.KrtOpts.ToOptions("TCPRouteV1")...,
			)
			tcpRouteCollections = append(tcpRouteCollections,
				krt.NewManyCollection(tcpRoutesV1, func(kctx krt.HandlerContext, i *gwv1.TCPRoute) []*gwv1a2.TCPRoute {
					if converted := convertTCPRouteV1ToV1Alpha2(i); converted != nil {
						return []*gwv1a2.TCPRoute{converted}
					}
					return nil
				}, c.KrtOpts.ToOptions("TCPRouteV1ToV1Alpha2")...))
		case gwv1a2.GroupVersion.Version:
			preV1TCPRoutes := krt.WrapClient(
				newGatedTypedInformer(ctx, tcpRouteGVR, tcpRouteGate(tcpRouteGVR), func() kclient.Informer[*gwv1a2.TCPRoute] {
					return kclient.NewFiltered[*gwv1a2.TCPRoute](c.Client, filter)
				}),
				c.KrtOpts.ToOptions("TCPRoutePreV1Alpha2")...,
			)
			tcpRouteCollections = append(tcpRouteCollections, preV1TCPRoutes)
		}
	}

	var tcproutes krt.Collection[*gwv1a2.TCPRoute]
	switch len(tcpRouteCollections) {
	case 0:
		tcproutes = krt.NewStaticCollection[*gwv1a2.TCPRoute](nil, nil, c.KrtOpts.ToOptions("disable/TCPRoute")...)
	case 1:
		tcproutes = tcpRouteCollections[0]
	default:
		tcproutes = krt.JoinCollection(tcpRouteCollections, c.KrtOpts.ToOptions("TCPRoute")...)
	}

	// As for TCPRoute above: one selection drives both the watches and the status writers.
	// TLSRoute is standard as of Gateway API v1.5; pre-v1 versions stay behind the
	// experimental feature flag.
	tlsRouteWriteGVRs := selectRouteGVRs(
		resolveRouteVersions(ctx, versionSource, wellknown.TLSRouteCRDName, tlsRouteGVRs),
		tlsRouteGVRs,
		includeLegacyRouteVersions,
	)
	tlsRouteGate := func(gvr schema.GroupVersionResource) informerGate {
		return routeInformerGate(versionSource, wellknown.TLSRouteCRDName, tlsRouteGVRs, includeLegacyRouteVersions, gvr)
	}
	tlsRouteCollections := make([]krt.Collection[*gwv1a2.TLSRoute], 0, len(tlsRouteWriteGVRs))
	for _, tlsRouteGVR := range tlsRouteWriteGVRs {
		switch tlsRouteGVR.Version {
		case gwv1.GroupVersion.Version:
			// A gated informer, not kclient.NewDelayedInformer, matching TCPRoute and the
			// pre-v1 TLSRoute paths. Istio's CRD watcher keys readiness on <resource>.<group>
			// and ignores the version, so on its own it would report v1 as ready off a CRD
			// serving no v1, start an informer against an unserved endpoint, and never sync —
			// blocking every collection gated on it. Istio only avoids that through
			// minimumVersionFilter, a hardcoded table whose tlsroutes minimum happens to be
			// the release where v1 appeared. Depending on that table staying aligned with
			// kgateway's needs is a coupling we do not need: the gate checks the served
			// version itself and keeps HasSynced unblocked when v1 is absent.
			tlsRoutesV1 := krt.WrapClient(
				newGatedTypedInformer(ctx, tlsRouteGVR, tlsRouteGate(tlsRouteGVR), func() kclient.Informer[*gwv1.TLSRoute] {
					return kclient.NewFiltered[*gwv1.TLSRoute](c.Client, filter)
				}),
				c.KrtOpts.ToOptions("TLSRouteV1")...,
			)
			tlsRouteCollections = append(tlsRouteCollections,
				krt.NewManyCollection(tlsRoutesV1, func(kctx krt.HandlerContext, i *gwv1.TLSRoute) []*gwv1a2.TLSRoute {
					if converted := convertTLSRouteV1ToV1Alpha2(i); converted != nil {
						return []*gwv1a2.TLSRoute{converted}
					}
					return nil
				}, c.KrtOpts.ToOptions("TLSRouteV1ToV1Alpha2")...))
		case wellknown.TLSRouteV1Alpha3Version:
			preV1TLSRoutes := krt.WrapClient(
				newGatedTypedInformer(ctx, tlsRouteGVR, tlsRouteGate(tlsRouteGVR), func() kclient.Informer[*gwv1a3.TLSRoute] {
					return kclient.NewFiltered[*gwv1a3.TLSRoute](c.Client, filter)
				}),
				c.KrtOpts.ToOptions("TLSRoutePreV1Alpha3")...,
			)
			tlsRouteCollections = append(tlsRouteCollections,
				krt.NewManyCollection(preV1TLSRoutes, func(kctx krt.HandlerContext, i *gwv1a3.TLSRoute) []*gwv1a2.TLSRoute {
					if converted := convertTLSRouteV1Alpha3ToV1Alpha2(i); converted != nil {
						return []*gwv1a2.TLSRoute{converted}
					}
					return nil
				}, c.KrtOpts.ToOptions("TLSRoutePreV1Alpha3ToV1Alpha2")...))
		case gwv1a2.GroupVersion.Version:
			preV1TLSRoutes := krt.WrapClient(
				newGatedTypedInformer(ctx, tlsRouteGVR, tlsRouteGate(tlsRouteGVR), func() kclient.Informer[*gwv1a2.TLSRoute] {
					return kclient.NewFiltered[*gwv1a2.TLSRoute](c.Client, filter)
				}),
				c.KrtOpts.ToOptions("TLSRoutePreV1Alpha2")...,
			)
			tlsRouteCollections = append(tlsRouteCollections, preV1TLSRoutes)
		}
	}

	var tlsRoutes krt.Collection[*gwv1a2.TLSRoute]
	switch len(tlsRouteCollections) {
	case 0:
		tlsRoutes = krt.NewStaticCollection[*gwv1a2.TLSRoute](nil, nil, c.KrtOpts.ToOptions("disable/TLSRoute")...)
	case 1:
		tlsRoutes = tlsRouteCollections[0]
	default:
		tlsRoutes = krt.JoinCollection(tlsRouteCollections, c.KrtOpts.ToOptions("TLSRoute")...)
	}
	metrics.RegisterEvents(tcproutes, kmetrics.GetResourceMetricEventHandler[*gwv1a2.TCPRoute]())
	metrics.RegisterEvents(tlsRoutes, kmetrics.GetResourceMetricEventHandler[*gwv1a2.TLSRoute]())

	grpcRoutes := krt.WrapClient(kclient.NewFilteredDelayed[*gwv1.GRPCRoute](c.Client, wellknown.GRPCRouteGVR, filter), c.KrtOpts.ToOptions("GRPCRoute")...)
	metrics.RegisterEvents(grpcRoutes, kmetrics.GetResourceMetricEventHandler[*gwv1.GRPCRoute]())

	c.RawHTTPRoutes = httpRoutes
	c.RawGRPCRoutes = grpcRoutes
	c.RawTCPRoutes = tcproutes
	c.RawTLSRoutes = tlsRoutes

	// The very lists the watches above were built from: a version we watch is a version we
	// can write, so the two cannot drift apart.
	c.tcpRouteWriteGVRs = tcpRouteWriteGVRs
	c.tlsRouteWriteGVRs = tlsRouteWriteGVRs

	backendIndex := krtcollections.NewBackendIndex(c.KrtOpts, policies, c.RefGrants)
	initBackends(plugins, backendIndex)
	endpointIRs := initEndpoints(plugins, c.KrtOpts)

	routes := krtcollections.NewRoutesIndex(c.KrtOpts, httpRoutes, grpcRoutes, tcproutes, tlsRoutes, policies, backendIndex, c.RefGrants, globalSettings)
	return gateways, routes, backendIndex, endpointIRs
}

func initBackends(plugins pluginsdk.Plugin, backendIndex *krtcollections.BackendIndex) {
	for gk, plugin := range plugins.ContributesBackends {
		if plugin.Backends != nil {
			backendIndex.AddBackends(gk, plugin.Backends, plugin.AliasKinds...)
		}
	}
}

func initEndpoints(plugins pluginsdk.Plugin, krtopts krtutil.KrtOptions) krt.Collection[ir.EndpointsForBackend] {
	allEndpoints := []krt.Collection[ir.EndpointsForBackend]{}
	for _, plugin := range plugins.ContributesBackends {
		if plugin.Endpoints != nil {
			allEndpoints = append(allEndpoints, plugin.Endpoints)
		}
	}
	// build Endpoint intermediate representation from kubernetes service and extensions
	// TODO move kube service to be an extension
	endpointIRs := krt.JoinCollection(allEndpoints, krtopts.ToOptions("EndpointIRs")...)
	return endpointIRs
}

func convertLegacyXListenerSetToV1(in *unstructured.Unstructured) *gwv1.ListenerSet {
	if in == nil {
		return nil
	}

	ls := &gwv1.ListenerSet{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(in.UnstructuredContent(), ls); err != nil {
		return nil
	}

	// Preserve the legacy GVK so downstream status/query code can distinguish old XListenerSets
	// from promoted ListenerSets after normalization.
	ls.SetGroupVersionKind(wellknown.XListenerSetGVK)
	return ls
}
