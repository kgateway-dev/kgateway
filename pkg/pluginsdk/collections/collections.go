package collections

import (
	"context"
	"fmt"

	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	"istio.io/istio/pkg/util/smallset"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

var logger = logging.New("pluginsdk/collections")

type CommonCollections struct {
	Client            apiclient.Client
	KrtOpts           krtutil.KrtOptions
	Secrets           *krtcollections.SecretIndex
	ConfigMaps        *krtcollections.ConfigMapIndex
	BackendIndex      *krtcollections.BackendIndex
	Routes            *krtcollections.RoutesIndex
	Namespaces        krt.Collection[krtcollections.NamespaceMetadata]
	Endpoints         krt.Collection[ir.EndpointsForBackend]
	GatewayIndex      *krtcollections.GatewayIndex
	GatewayExtensions krt.Collection[ir.GatewayExtension]
	Services          krt.Collection[*corev1.Service]
	ServiceEntries    krt.Collection[*networkingclient.ServiceEntry]

	// ServiceEntriesExclusionLabelSelectors is parsed from Settings.ServiceEntriesExclusionLabelSelectors.
	// Keep it with CommonCollections so ServiceEntry exclusion config has one validated source of truth.
	ServiceEntriesExclusionLabelSelectors []labels.Selector

	WrappedPods  krt.Collection[krtcollections.WrappedPod]
	LocalityPods krt.Collection[krtcollections.LocalityPod]
	RefGrants    *krtcollections.RefGrantIndex

	// Raw informer-backed collections of the Gateway API resources kgateway writes status
	// for. These share informers with the IR collections above; they are exposed so the
	// status syncer can derive per-object desired-status collections that see live status
	// changes (the IR collections may not re-emit on status-only updates).
	// TCP and TLS routes are normalized to their v1alpha2 representation; legacy
	// XListenerSets are normalized to gwv1.ListenerSet with their GroupVersionKind
	// preserved as XListenerSet.
	RawGateways     krt.Collection[*gwv1.Gateway]
	RawListenerSets krt.Collection[*gwv1.ListenerSet]
	RawHTTPRoutes   krt.Collection[*gwv1.HTTPRoute]
	RawGRPCRoutes   krt.Collection[*gwv1.GRPCRoute]
	RawTCPRoutes    krt.Collection[*gwv1a2.TCPRoute]
	RawTLSRoutes    krt.Collection[*gwv1a2.TLSRoute]

	// tcpRouteWriteGVRs and tlsRouteWriteGVRs identify the served API versions status
	// writes may go through, most preferred first, resolved from CRD discovery at startup.
	// Normally one entry. More than one means discovery was not authoritative and the
	// writer must dispatch to whichever version's informer actually holds the object.
	// Read them through TCPRouteWriteVersions/TLSRouteWriteVersions, which supply the
	// fallback for a CommonCollections built without InitCollections.
	tcpRouteWriteGVRs []schema.GroupVersionResource
	tlsRouteWriteGVRs []schema.GroupVersionResource

	DiscoveryNamespacesFilter kubetypes.DynamicObjectFilter

	// static set of global Settings, non-krt based for dev speed
	// TODO: this should be refactored to a more correct location,
	// or even better, be removed entirely and done per Gateway (maybe in GwParams)
	Settings       apisettings.Settings
	ControllerName string

	options *option
}

// WatchLabelSelector returns the label selector that a watch in the given discovery mode
// must push to the API server, or the empty string for an unfiltered watch.
func WatchLabelSelector(mode apisettings.DiscoveryMode) string {
	if mode != apisettings.DiscoveryLabeled {
		return ""
	}
	return wellknown.WatchLabel + "=" + wellknown.WatchLabelValue
}

// TCPRouteWriteVersions returns the served TCPRoute API versions status writes may go
// through, most preferred first. It never returns an empty slice, so callers can index the
// preferred version unconditionally: a CommonCollections built without InitCollections
// (which some tests do) would otherwise leave the kind with no write version at all.
func (c *CommonCollections) TCPRouteWriteVersions() []schema.GroupVersionResource {
	return routeWriteVersionsOrDefault(c.tcpRouteWriteGVRs, wellknown.TCPRouteV1GVR)
}

// TLSRouteWriteVersions is TCPRouteWriteVersions for TLSRoutes.
func (c *CommonCollections) TLSRouteWriteVersions() []schema.GroupVersionResource {
	return routeWriteVersionsOrDefault(c.tlsRouteWriteGVRs, wellknown.TLSRouteV1GVR)
}

func routeWriteVersionsOrDefault(gvrs []schema.GroupVersionResource, fallback schema.GroupVersionResource) []schema.GroupVersionResource {
	if len(gvrs) == 0 {
		return []schema.GroupVersionResource{fallback}
	}
	return gvrs
}

func (c *CommonCollections) HasSynced() bool {
	// we check nil as well because some of the inner
	// collections aren't initialized until we call InitPlugins
	return c.Secrets != nil && c.Secrets.HasSynced() &&
		c.ConfigMaps != nil && c.ConfigMaps.HasSynced() &&
		c.BackendIndex != nil && c.BackendIndex.HasSynced() &&
		c.Routes != nil && c.Routes.HasSynced() &&
		c.WrappedPods != nil && c.WrappedPods.HasSynced() &&
		c.LocalityPods != nil && c.LocalityPods.HasSynced() &&
		c.RefGrants != nil && c.RefGrants.HasSynced() &&
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
	settings apisettings.Settings,
	opts ...Option,
) (*CommonCollections, error) {
	options := &option{}
	for _, fn := range opts {
		fn(options)
	}

	// Namespace collection must be initialized first to enable discovery namespace
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

	// In LABELED mode the watch label is pushed to the API server as a watch selector, so
	// objects kgateway is not expected to resolve never reach the informer cache.
	secretWatchSelector := WatchLabelSelector(settings.SecretDiscoveryMode)
	configMapWatchSelector := WatchLabelSelector(settings.ConfigMapDiscoveryMode)
	if secretWatchSelector != "" {
		logger.Info("watching only labeled Secrets", "selector", secretWatchSelector)
	}
	if configMapWatchSelector != "" {
		logger.Info("watching only labeled ConfigMaps", "selector", configMapWatchSelector)
	}

	secretClient := kclient.NewFiltered[*corev1.Secret](
		client,
		kclient.Filter{
			FieldSelector: apiclient.SecretsFieldSelector,
			LabelSelector: secretWatchSelector,
			ObjectFilter:  client.ObjectFilter(),
		},
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
	refgrants := krtcollections.NewRefGrantIndex(refgrantsCol, settings.ReferenceGrantMode)

	// The gateway controller opens a Service client of its own (see gw_controller.go) and
	// applies this same selector, so the two share one informer.
	serviceClient := kclient.NewFiltered[*corev1.Service](
		client,
		kclient.Filter{
			LabelSelector: WatchLabelSelector(settings.ServiceDiscoveryMode),
			ObjectFilter:  client.ObjectFilter(),
		},
	)
	services := krt.WrapClient(serviceClient, krtOptions.ToOptions("Services")...)

	var serviceEntries krt.Collection[*networkingclient.ServiceEntry]
	var serviceEntriesExclusionLabelSelectors []labels.Selector
	if settings.EnableIstioIntegration {
		var err error
		serviceEntriesExclusionLabelSelectors, err = ParseExclusionLabelSelectors(settings.ServiceEntriesExclusionLabelSelectors)
		if err != nil {
			return nil, fmt.Errorf("error parsing ServiceEntry exclusion label selectors: %w", err)
		}
		seInformer := kclient.NewDelayedInformer[*networkingclient.ServiceEntry](
			client, gvr.ServiceEntry,
			kubetypes.StandardInformer, kclient.Filter{ObjectFilter: client.ObjectFilter()},
		)
		serviceEntries = krt.WrapClient(seInformer, krtOptions.ToOptions("ServiceEntries")...)
	} else {
		serviceEntries = krt.NewStaticCollection[*networkingclient.ServiceEntry](nil, nil, krtOptions.ToOptions("disable/ServiceEntries")...)
	}

	cmClient := kclient.NewFiltered[*corev1.ConfigMap](
		client,
		kclient.Filter{
			LabelSelector: configMapWatchSelector,
			ObjectFilter:  client.ObjectFilter(),
		},
	)
	cfgmaps := krt.WrapClient(cmClient, krtOptions.ToOptions("ConfigMaps")...)

	gwExts := krtcollections.NewGatewayExtensionsCollection(ctx, client, krtOptions)

	localityPods, wrappedPods := krtcollections.NewPodsCollection(client, krtOptions)

	return &CommonCollections{
		Client:                                client,
		KrtOpts:                               krtOptions,
		Secrets:                               krtcollections.NewSecretIndex(secrets, refgrants),
		ConfigMaps:                            krtcollections.NewConfigMapIndex(cfgmaps, refgrants),
		LocalityPods:                          localityPods,
		WrappedPods:                           wrappedPods,
		RefGrants:                             refgrants,
		Settings:                              settings,
		Namespaces:                            namespaces,
		Services:                              services,
		ServiceEntries:                        serviceEntries,
		ServiceEntriesExclusionLabelSelectors: serviceEntriesExclusionLabelSelectors,
		GatewayExtensions:                     gwExts,

		DiscoveryNamespacesFilter: discoveryNamespacesFilter,

		ControllerName: controllerName,

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
		smallset.New(c.ControllerName),
		mergedPlugins,
		globalSettings,
	)

	// init plugin-extended collections
	c.BackendIndex = backendIndex
	c.Routes = routeIndex
	c.Endpoints = endpointIRs
	c.GatewayIndex = gateways
}
