package collections

import (
	"context"

	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	"istio.io/istio/pkg/util/smallset"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	kmetrics "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

type CommonCollections struct {
	Client            apiclient.Client
	KrtOpts           krtutil.KrtOptions
	Secrets           *krtcollections.SecretIndex
	BackendIndex      *krtcollections.BackendIndex
	Routes            *krtcollections.RoutesIndex
	Namespaces        krt.Collection[krtcollections.NamespaceMetadata]
	Endpoints         krt.Collection[ir.EndpointsForBackend]
	GatewayIndex      *krtcollections.GatewayIndex
	GatewayExtensions krt.Collection[ir.GatewayExtension]
	Services          krt.Collection[*corev1.Service]
	ServiceEntries    krt.Collection[*networkingclient.ServiceEntry]

	WrappedPods  krt.Collection[krtcollections.WrappedPod]
	LocalityPods krt.Collection[krtcollections.LocalityPod]
	RefGrants    *krtcollections.RefGrantIndex
	ConfigMaps   krt.Collection[*corev1.ConfigMap]

	DiscoveryNamespacesFilter kubetypes.DynamicObjectFilter

	// static set of global Settings, non-krt based for dev speed
	// TODO: this should be refactored to a more correct location,
	// or even better, be removed entirely and done per Gateway (maybe in GwParams)
	Settings                   apisettings.Settings
	ControllerName             string
	AgentgatewayControllerName string

	options *option
}

func (c *CommonCollections) HasSynced() bool {
	// we check nil as well because some of the inner
	// collections aren't initialized until we call InitPlugins
	return c.Secrets != nil && c.Secrets.HasSynced() &&
		c.BackendIndex != nil && c.BackendIndex.HasSynced() &&
		c.Routes != nil && c.Routes.HasSynced() &&
		c.WrappedPods != nil && c.WrappedPods.HasSynced() &&
		c.LocalityPods != nil && c.LocalityPods.HasSynced() &&
		c.RefGrants != nil && c.RefGrants.HasSynced() &&
		c.ConfigMaps != nil && c.ConfigMaps.HasSynced() &&
		c.GatewayExtensions != nil && c.GatewayExtensions.HasSynced() &&
		c.Services != nil && c.Services.HasSynced() &&
		c.ServiceEntries != nil && c.ServiceEntries.HasSynced() &&
		c.GatewayIndex != nil && c.GatewayIndex.Gateways.HasSynced()
}

// NewCommonCollections initializes the core krt collections.
// Collections that rely on plugins aren't initialized here,
// and InitPlugins must be called.
func NewCommonCollections(
	ctx context.Context,
	krtOptions krtutil.KrtOptions,
	client apiclient.Client,
	controllerName string,
	agentGatewayControllerName string,
	settings apisettings.Settings,
	opts ...Option,
) (*CommonCollections, error) {
	options := &option{}
	for _, fn := range opts {
		fn(options)
	} // Namespace collection must be initialized first to enable discovery namespace
	// selectors to be applies as filters to other collections
	namespaces, nsClient := krtcollections.NewNamespaceCollection(ctx, client, krtOptions)

	var err error
	// Initialize discovery namespace filter if it has not already been set.
	// We should not overwrite an existing filter as it may have been set up with a custom apiclient.Client
	discoveryNamespacesFilter := client.ObjectFilter()
	if discoveryNamespacesFilter == nil {
		discoveryNamespacesFilter, err = NewDiscoveryNamespacesFilter(nsClient, settings.DiscoveryNamespaceSelectors, ctx.Done())
		if err != nil {
			return nil, err
		}
		kube.SetObjectFilter(client.Core(), discoveryNamespacesFilter)
	}

	secretClient := kclient.NewFiltered[*corev1.Secret](
		client,
		kclient.Filter{ObjectFilter: client.ObjectFilter()},
	)
	k8sSecretsRaw := krt.WrapClient(secretClient, krt.WithStop(krtOptions.Stop), krt.WithName("Secrets") /* no debug here - we don't want raw secrets printed*/)
	k8sSecrets := krt.NewCollection(k8sSecretsRaw, func(kctx krt.HandlerContext, i *corev1.Secret) *ir.Secret {
		res := ir.Secret{
			ObjectSource: ir.ObjectSource{
				Group:     "",
				Kind:      "Secret",
				Namespace: i.Namespace,
				Name:      i.Name,
			},
			Obj:  i,
			Data: i.Data,
		}
		return &res
	}, krtOptions.ToOptions("secrets")...)
	secrets := map[schema.GroupKind]krt.Collection[ir.Secret]{
		{Group: "", Kind: "Secret"}: k8sSecrets,
	}

	refgrantsCol := krt.WrapClient(kclient.NewFilteredDelayed[*gwv1b1.ReferenceGrant](
		client,
		wellknown.ReferenceGrantGVR,
		kclient.Filter{ObjectFilter: client.ObjectFilter()},
	), krtOptions.ToOptions("RefGrants")...)
	refgrants := krtcollections.NewRefGrantIndex(refgrantsCol)

	serviceClient := kclient.NewFiltered[*corev1.Service](
		client,
		kclient.Filter{ObjectFilter: client.ObjectFilter()},
	)
	services := krt.WrapClient(serviceClient, krtOptions.ToOptions("Services")...)

	seInformer := kclient.NewDelayedInformer[*networkingclient.ServiceEntry](
		client, gvr.ServiceEntry,
		kubetypes.StandardInformer, kclient.Filter{ObjectFilter: client.ObjectFilter()},
	)
	serviceEntries := krt.WrapClient(seInformer, krtOptions.ToOptions("ServiceEntries")...)

	cmClient := kclient.NewFiltered[*corev1.ConfigMap](
		client,
		kclient.Filter{ObjectFilter: client.ObjectFilter()},
	)
	cfgmaps := krt.WrapClient(cmClient, krtOptions.ToOptions("ConfigMaps")...)

	gwExts := krtcollections.NewGatewayExtensionsCollection(ctx, client, krtOptions)

	localityPods, wrappedPods := krtcollections.NewPodsCollection(client, krtOptions)

	return &CommonCollections{
		Client:            client,
		KrtOpts:           krtOptions,
		Secrets:           krtcollections.NewSecretIndex(secrets, refgrants),
		LocalityPods:      localityPods,
		WrappedPods:       wrappedPods,
		RefGrants:         refgrants,
		Settings:          settings,
		Namespaces:        namespaces,
		Services:          services,
		ServiceEntries:    serviceEntries,
		ConfigMaps:        cfgmaps,
		GatewayExtensions: gwExts,

		DiscoveryNamespacesFilter: discoveryNamespacesFilter,

		ControllerName:             controllerName,
		AgentgatewayControllerName: agentGatewayControllerName,

		options: options,
	}, nil
}

// InitPlugins set up collections that rely on plugins.
// This can't be part of NewCommonCollections because the setup
// of plugins themselves rely on a reference to CommonCollections.
func (c *CommonCollections) InitPlugins(
	ctx context.Context,
	mergedPlugins pluginsdk.Plugin,
	globalSettings apisettings.Settings,
) {
	gateways, routeIndex, backendIndex, endpointIRs := c.InitCollections(
		ctx,
		smallset.New(c.ControllerName, c.AgentgatewayControllerName),
		mergedPlugins,
		globalSettings,
	)

	// init plugin-extended collections
	c.BackendIndex = backendIndex
	c.Routes = routeIndex
	c.Endpoints = endpointIRs
	c.GatewayIndex = gateways
}

func (c *CommonCollections) InitCollections(
	ctx context.Context,
	controllerNames smallset.Set[string],
	plugins pluginsdk.Plugin,
	globalSettings apisettings.Settings,
) (*krtcollections.GatewayIndex, *krtcollections.RoutesIndex, *krtcollections.BackendIndex, krt.Collection[ir.EndpointsForBackend]) {
	// discovery filter
	filter := kclient.Filter{ObjectFilter: c.Client.ObjectFilter()}

	//nolint:forbidigo // ObjectFilter is not needed for this client as it is cluster scoped
	gatewayClasses := krt.WrapClient(kclient.New[*gwv1.GatewayClass](c.Client), c.KrtOpts.ToOptions("KubeGatewayClasses")...)

	namespaces, _ := krtcollections.NewNamespaceCollection(ctx, c.Client, c.KrtOpts)

	kubeRawGateways := krt.WrapClient(kclient.NewFilteredDelayed[*gwv1.Gateway](c.Client, wellknown.GatewayGVR, filter), c.KrtOpts.ToOptions("KubeGateways")...)
	metrics.RegisterEvents(kubeRawGateways, kmetrics.GetResourceMetricEventHandler[*gwv1.Gateway]())

	var kubeRawListenerSets krt.Collection[*gwxv1a1.XListenerSet]
	// ON_EXPERIMENTAL_PROMOTION : Remove this block
	// Ref: https://github.com/kgateway-dev/kgateway/issues/12827
	if globalSettings.EnableExperimentalGatewayAPIFeatures {
		kubeRawListenerSets = krt.WrapClient(kclient.NewDelayedInformer[*gwxv1a1.XListenerSet](c.Client, wellknown.XListenerSetGVR, kubetypes.StandardInformer, filter), c.KrtOpts.ToOptions("KubeListenerSets")...)
	} else {
		// If disabled, still build a collection but make it always empty
		kubeRawListenerSets = krt.NewStaticCollection[*gwxv1a1.XListenerSet](nil, nil, c.KrtOpts.ToOptions("disable/KubeListenerSets")...)
	}
	metrics.RegisterEvents(kubeRawListenerSets, kmetrics.GetResourceMetricEventHandler[*gwxv1a1.XListenerSet]())

	var policies *krtcollections.PolicyIndex
	if globalSettings.EnableEnvoy {
		policies = krtcollections.NewPolicyIndex(c.KrtOpts, plugins.ContributesPolicies, globalSettings)
		for _, plugin := range plugins.ContributesPolicies {
			if plugin.Policies != nil {
				metrics.RegisterEvents(plugin.Policies, kmetrics.GetResourceMetricEventHandler[ir.PolicyWrapper]())
			}
		}
	}

	gateways := krtcollections.NewGatewayIndex(krtcollections.NewGatewayIndexConfig(
		c.KrtOpts,
		controllerNames,
		c.ControllerName,
		policies,
		kubeRawGateways,
		kubeRawListenerSets,
		gatewayClasses,
		namespaces,
		krtcollections.WithGatewayForDeployerTransformationFunc(c.options.gatewayForDeployerTransformationFunc),
		krtcollections.WithGatewayForEnvoyTransformationFunc(c.options.gatewayForEnvoyTransformationFunc),
	))

	if !globalSettings.EnableEnvoy {
		// For now, the gateway index is used by Agentgateway as well in the deployer
		return gateways, nil, nil, nil
	}

	// create the KRT clients, remember to also register any needed types in the type registration setup.
	httpRoutes := krt.WrapClient(kclient.NewFilteredDelayed[*gwv1.HTTPRoute](c.Client, wellknown.HTTPRouteGVR, filter), c.KrtOpts.ToOptions("HTTPRoute")...)
	metrics.RegisterEvents(httpRoutes, kmetrics.GetResourceMetricEventHandler[*gwv1.HTTPRoute]())

	// ON_EXPERIMENTAL_PROMOTION : Remove this block
	// Ref: https://github.com/kgateway-dev/kgateway/issues/12879
	var tcproutes krt.Collection[*gwv1a2.TCPRoute]
	// Ref: https://github.com/kgateway-dev/kgateway/issues/12880
	var tlsRoutes krt.Collection[*gwv1a2.TLSRoute]
	if globalSettings.EnableExperimentalGatewayAPIFeatures {
		tcproutes = krt.WrapClient(kclient.NewDelayedInformer[*gwv1a2.TCPRoute](c.Client, gvr.TCPRoute, kubetypes.StandardInformer, filter), c.KrtOpts.ToOptions("TCPRoute")...)
		tlsRoutes = krt.WrapClient(kclient.NewDelayedInformer[*gwv1a2.TLSRoute](c.Client, gvr.TLSRoute, kubetypes.StandardInformer, filter), c.KrtOpts.ToOptions("TLSRoute")...)
	} else {
		// If disabled, still build a collection but make it always empty
		tcproutes = krt.NewStaticCollection[*gwv1a2.TCPRoute](nil, nil, c.KrtOpts.ToOptions("disable/TCPRoute")...)
		tlsRoutes = krt.NewStaticCollection[*gwv1a2.TLSRoute](nil, nil, c.KrtOpts.ToOptions("disable/TLSRoute")...)
	}
	metrics.RegisterEvents(tcproutes, kmetrics.GetResourceMetricEventHandler[*gwv1a2.TCPRoute]())
	metrics.RegisterEvents(tlsRoutes, kmetrics.GetResourceMetricEventHandler[*gwv1a2.TLSRoute]())

	grpcRoutes := krt.WrapClient(kclient.NewFilteredDelayed[*gwv1.GRPCRoute](c.Client, wellknown.GRPCRouteGVR, filter), c.KrtOpts.ToOptions("GRPCRoute")...)
	metrics.RegisterEvents(grpcRoutes, kmetrics.GetResourceMetricEventHandler[*gwv1.GRPCRoute]())

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
