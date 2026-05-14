package collections

import (
	"context"

	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	"istio.io/istio/pkg/util/smallset"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
			newDelayedDynamicUnstructuredInformer(c.Client, wellknown.XListenerSetGVR, filter),
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

	var policies *krtcollections.PolicyIndex
	if globalSettings.EnableEnvoy {
		policies = krtcollections.NewPolicyIndex(c.KrtOpts, plugins.ContributesPolicies, globalSettings)
		for _, plugin := range plugins.ContributesPolicies {
			if plugin.Policies != nil {
				metrics.RegisterEvents(plugin.Policies, kmetrics.GetResourceMetricEventHandler[ir.PolicyWrapper]())
			}
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

	if !globalSettings.EnableEnvoy {
		// When Envoy is disabled, only the gateway index is needed for the deployer
		return gateways, nil, nil, nil
	}

	// create the KRT clients, remember to also register any needed types in the type registration setup.
	httpRoutes := krt.WrapClient(kclient.NewFilteredDelayed[*gwv1.HTTPRoute](c.Client, wellknown.HTTPRouteGVR, filter), c.KrtOpts.ToOptions("HTTPRoute")...)
	metrics.RegisterEvents(httpRoutes, kmetrics.GetResourceMetricEventHandler[*gwv1.HTTPRoute]())

	// ON_EXPERIMENTAL_PROMOTION : Remove this block
	// Ref: https://github.com/kgateway-dev/kgateway/issues/12879
	var tcproutes krt.Collection[*gwv1a2.TCPRoute]
	if globalSettings.EnableExperimentalGatewayAPIFeatures {
		tcproutes = krt.WrapClient(
			newDelayedTypedInformer(c.Client, gvr.TCPRoute, func() kclient.Informer[*gwv1a2.TCPRoute] {
				return kclient.NewFiltered[*gwv1a2.TCPRoute](c.Client, filter)
			}),
			c.KrtOpts.ToOptions("TCPRoute")...,
		)
	} else {
		// If disabled, still build a collection but make it always empty
		tcproutes = krt.NewStaticCollection[*gwv1a2.TCPRoute](nil, nil, c.KrtOpts.ToOptions("disable/TCPRoute")...)
	}

	// TLSRoute is standard as of Gateway API v1.5, so promoted v1 TLSRoutes
	// are always enabled. Keep pre-v1 TLSRoute watches under the experimental
	// feature flag for compatibility with older Gateway API channels.
	servedTLSRouteVersions := getServedTLSRouteVersions(c.Client.Ext())
	tlsRoutesV1 := krt.WrapClient(
		kclient.NewDelayedInformer[*gwv1.TLSRoute](c.Client, promotedTLSRouteGVR, kubetypes.StandardInformer, filter),
		c.KrtOpts.ToOptions("TLSRouteV1")...,
	)
	tlsRouteCollections := []krt.Collection[*gwv1a2.TLSRoute]{
		krt.NewManyCollection(tlsRoutesV1, func(kctx krt.HandlerContext, i *gwv1.TLSRoute) []*gwv1a2.TLSRoute {
			if converted := convertTLSRouteV1ToV1Alpha2(i); converted != nil {
				return []*gwv1a2.TLSRoute{converted}
			}
			return nil
		}, c.KrtOpts.ToOptions("TLSRouteV1ToV1Alpha2")...),
	}
	if globalSettings.EnableExperimentalGatewayAPIFeatures {
		for _, preV1TLSRouteGVR := range preV1TLSRouteWatchGVRs(servedTLSRouteVersions) {
			switch preV1TLSRouteGVR.Version {
			case gwv1a2.GroupVersion.Version:
				preV1TLSRoutes := krt.WrapClient(
					newDelayedTypedInformer(c.Client, preV1TLSRouteGVR, func() kclient.Informer[*gwv1a2.TLSRoute] {
						return kclient.NewFiltered[*gwv1a2.TLSRoute](c.Client, filter)
					}),
					c.KrtOpts.ToOptions("TLSRoutePreV1Alpha2")...,
				)
				tlsRouteCollections = append(tlsRouteCollections, preV1TLSRoutes)
			case wellknown.TLSRouteV1Alpha3Version:
				preV1TLSRoutes := krt.WrapClient(
					newDelayedTypedInformer(c.Client, preV1TLSRouteGVR, func() kclient.Informer[*gwv1a3.TLSRoute] {
						return kclient.NewFiltered[*gwv1a3.TLSRoute](c.Client, filter)
					}),
					c.KrtOpts.ToOptions("TLSRoutePreV1Alpha3")...,
				)
				tlsRouteCollections = append(tlsRouteCollections, krt.NewManyCollection(preV1TLSRoutes, func(kctx krt.HandlerContext, i *gwv1a3.TLSRoute) []*gwv1a2.TLSRoute {
					if converted := convertTLSRouteV1Alpha3ToV1Alpha2(i); converted != nil {
						return []*gwv1a2.TLSRoute{converted}
					}
					return nil
				}, c.KrtOpts.ToOptions("TLSRoutePreV1Alpha3ToV1Alpha2")...))
			}
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

	backendIndex := krtcollections.NewBackendIndex(c.KrtOpts, policies, c.RefGrants)
	initBackends(plugins, backendIndex)
	endpointIRs := initEndpoints(plugins, c.KrtOpts)

	routes := krtcollections.NewRoutesIndex(c.KrtOpts, c.ControllerName, httpRoutes, grpcRoutes, tcproutes, tlsRoutes, policies, backendIndex, c.RefGrants, globalSettings)
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
