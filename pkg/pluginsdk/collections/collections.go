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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/ondemand"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

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

	// secretCache and configMapCache back Secrets and ConfigMaps. They hold the
	// contents of referenced objects only; the cluster-wide view they maintain is
	// metadata-only. Their reference inputs are attached in InitPlugins, once the
	// plugins that declare those references exist.
	secretCache    *ondemand.Cache[*corev1.Secret]
	configMapCache *ondemand.Cache[*corev1.ConfigMap]

	// coreResourceRefs are the references core Gateway API translation produces.
	// Set by InitCollections.
	coreResourceRefs krt.Collection[ondemand.ResourceRef]

	// gwExtResourceRefs are the references GatewayExtension-backed policies read.
	gwExtResourceRefs krt.Collection[ondemand.ResourceRef]

	DiscoveryNamespacesFilter kubetypes.DynamicObjectFilter

	// static set of global Settings, non-krt based for dev speed
	// TODO: this should be refactored to a more correct location,
	// or even better, be removed entirely and done per Gateway (maybe in GwParams)
	Settings       apisettings.Settings
	ControllerName string

	options *option
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

	// Secrets and ConfigMaps are watched metadata-only cluster-wide; only objects
	// some part of the configuration references have their contents fetched. See
	// the ondemand package for why.
	secretCache := ondemand.New(ctx, client, krtOptions, ondemand.Config[*corev1.Secret]{
		Name: "Secrets",
		Kind: wellknown.SecretKind,
		GVR:  gvr.Secret,
		Filter: kclient.Filter{
			FieldSelector: apiclient.SecretsFieldSelector,
			ObjectFilter:  client.ObjectFilter(),
		},
		Getter: func(ctx context.Context, ns, name string) (*corev1.Secret, error) {
			return client.Kube().CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
		},
	})
	// no debug here - we don't want raw secrets printed
	k8sSecretsRaw := secretCache.Collection()
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

	serviceClient := kclient.NewFiltered[*corev1.Service](
		client,
		kclient.Filter{ObjectFilter: client.ObjectFilter()},
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

	configMapCache := ondemand.New(ctx, client, krtOptions, ondemand.Config[*corev1.ConfigMap]{
		Name:   "ConfigMaps",
		Kind:   wellknown.ConfigMapKind,
		GVR:    gvr.ConfigMap,
		Filter: kclient.Filter{ObjectFilter: client.ObjectFilter()},
		Getter: func(ctx context.Context, ns, name string) (*corev1.ConfigMap, error) {
			return client.Kube().CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
		},
	})
	cfgmaps := configMapCache.Collection()

	gwExts, rawGwExts := krtcollections.NewGatewayExtensionsCollection(ctx, client, krtOptions)

	localityPods, wrappedPods := krtcollections.NewPodsCollection(client, krtOptions)

	return &CommonCollections{
		Client:                                client,
		KrtOpts:                               krtOptions,
		Secrets:                               krtcollections.NewSecretIndex(secrets, refgrants).WithExistenceCheck(secretCache.Exists),
		ConfigMaps:                            krtcollections.NewConfigMapIndex(cfgmaps, refgrants).WithExistenceCheck(configMapCache.Exists),
		LocalityPods:                          localityPods,
		WrappedPods:                           wrappedPods,
		RefGrants:                             refgrants,
		Settings:                              settings,
		Namespaces:                            namespaces,
		Services:                              services,
		ServiceEntries:                        serviceEntries,
		ServiceEntriesExclusionLabelSelectors: serviceEntriesExclusionLabelSelectors,
		GatewayExtensions:                     gwExts,

		gwExtResourceRefs: krtcollections.GatewayExtensionResourceRefs(rawGwExts, krtOptions),

		secretCache:    secretCache,
		configMapCache: configMapCache,

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

	// Now that every plugin exists, assemble the full set of Secret/ConfigMap
	// references and start fetching. Until this runs the caches report unsynced,
	// so nothing translates against an empty Secrets collection.
	refs := c.buildResourceRefs(mergedPlugins)
	c.secretCache.SetRefs(ctx, refs)
	c.configMapCache.SetRefs(ctx, refs)
}

// buildResourceRefs unions the references contributed by core translation with
// those declared by each plugin.
func (c *CommonCollections) buildResourceRefs(
	mergedPlugins pluginsdk.Plugin,
) krt.Collection[ondemand.ResourceRef] {
	var cols []krt.Collection[ondemand.ResourceRef]
	if c.coreResourceRefs != nil {
		cols = append(cols, c.coreResourceRefs)
	}
	if c.gwExtResourceRefs != nil {
		cols = append(cols, c.gwExtResourceRefs)
	}
	cols = append(cols, mergedPlugins.ContributesResourceRefs...)

	return krt.JoinCollection(cols, c.KrtOpts.ToOptions("ResourceRefs")...)
}
