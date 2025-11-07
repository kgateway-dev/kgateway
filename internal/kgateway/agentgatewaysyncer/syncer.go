package agentgatewaysyncer

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/agentgateway/agentgateway/go/api"
	envoytypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"
	"istio.io/istio/pkg/util/sets"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayx "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer/krtxds"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer/nack"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer/status"
	agwir "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

var (
	logger                                = logging.New("agentgateway/syncer")
	_      manager.LeaderElectionRunnable = &Syncer{}
)

// Syncer synchronizes Kubernetes Gateway API resources with xDS for agentgateway proxies.
// It watches Gateway resources with the agentgateway class and translates them to agentgateway configuration.
type Syncer struct {
	// Core collections and dependencies
	agwCollections *plugins.AgwCollections
	client         kube.Client
	agwPlugins     plugins.AgwPlugin
	translator     *translator.AgwTranslator

	// Configuration
	controllerName           string
	additionalGatewayClasses map[string]*deployer.GatewayClassInfo

	// Status reporting
	statusCollections *status.StatusCollections

	// Synchronization
	waitForSync []cache.InformerSynced
	ready       atomic.Bool

	// NACK handling
	EventPublisher *nack.NackEventPublisher

	// features
	Registrations []krtxds.Registration
}

func NewAgwSyncer(
	ctx context.Context,
	controllerName string,
	client kube.Client,
	agwCollections *plugins.AgwCollections,
	agwPlugins plugins.AgwPlugin,
	additionalGatewayClasses map[string]*deployer.GatewayClassInfo,
) *Syncer {
	return &Syncer{
		agwCollections:           agwCollections,
		controllerName:           controllerName,
		agwPlugins:               agwPlugins,
		translator:               translator.NewAgwTranslator(agwCollections),
		additionalGatewayClasses: additionalGatewayClasses,
		client:                   client,
		statusCollections:        &status.StatusCollections{},
		EventPublisher:           nack.NewNackEventPublisher(ctx, client),
	}
}

func (s *Syncer) Init(krtopts krtutil.KrtOptions) {
	logger.Debug("init agentgateway Syncer", "controllername", s.controllerName)

	s.translator.Init()
	s.buildResourceCollections(krtopts)
}

func (s *Syncer) StatusCollections() *status.StatusCollections {
	return s.statusCollections
}

func (s *Syncer) buildResourceCollections(krtopts krtutil.KrtOptions) {
	// Build core collections for irs
	gatewayClasses := translator.GatewayClassesCollection(s.agwCollections.GatewayClasses, krtopts)
	refGrants := translator.BuildReferenceGrants(translator.ReferenceGrantsCollection(s.agwCollections.ReferenceGrants, krtopts))
	listenerSetStatus, listenerSets := s.buildListenerSetCollection(gatewayClasses, refGrants, krtopts)
	status.RegisterStatus(s.statusCollections, listenerSetStatus, translator.GetStatus)
	gatewayInitialStatus, gateways := s.buildGatewayCollection(gatewayClasses, listenerSets, refGrants, krtopts)

	// Build Agw resources for gateway
	agwResources, routeAttachments, policyStatuses := s.buildAgwResources(gateways, refGrants, krtopts)
	for _, col := range policyStatuses {
		status.RegisterStatus(s.statusCollections, col, translator.GetStatus)
	}

	gatewayFinalStatus := s.buildFinalGatewayStatus(gatewayInitialStatus, routeAttachments, krtopts)
	status.RegisterStatus(s.statusCollections, gatewayFinalStatus, translator.GetStatus)

	// Build address collections
	addresses := s.buildAddressCollections(krtopts)

	// Build XDS collection
	s.buildXDSCollection(agwResources, addresses, krtopts)

	// Set up sync dependencies
	s.setupSyncDependencies(agwResources, addresses)
}

func (s *Syncer) buildFinalGatewayStatus(
	gatewayStatuses krt.StatusCollection[*gwv1.Gateway, gwv1.GatewayStatus],
	routeAttachments krt.Collection[*translator.RouteAttachment],
	krtopts krtutil.KrtOptions,
) krt.StatusCollection[*gwv1.Gateway, gwv1.GatewayStatus] {
	routeAttachmentsIndex := krt.NewIndex(routeAttachments, "to", func(o *translator.RouteAttachment) []types.NamespacedName {
		return []types.NamespacedName{o.To}
	})
	return krt.NewCollection(
		gatewayStatuses,
		func(ctx krt.HandlerContext, i krt.ObjectWithStatus[*gwv1.Gateway, gwv1.GatewayStatus]) *krt.ObjectWithStatus[*gwv1.Gateway, gwv1.GatewayStatus] {
			tcpRoutes := krt.Fetch(ctx, routeAttachments, krt.FilterIndex(routeAttachmentsIndex, config.NamespacedName(i.Obj)))
			counts := map[string]int32{}
			for _, r := range tcpRoutes {
				counts[r.ListenerName]++
			}
			status := i.Status.DeepCopy()
			for i, s := range status.Listeners {
				s.AttachedRoutes = counts[string(s.Name)]
				status.Listeners[i] = s
			}
			return &krt.ObjectWithStatus[*gwv1.Gateway, gwv1.GatewayStatus]{
				Obj:    i.Obj,
				Status: *status,
			}
		}, krtopts.ToOptions("GatewayFinalStatus")...)
}

func (s *Syncer) buildGatewayCollection(
	gatewayClasses krt.Collection[translator.GatewayClass],
	listenerSets krt.Collection[translator.ListenerSet],
	refGrants translator.ReferenceGrants,
	krtopts krtutil.KrtOptions,
) (
	krt.StatusCollection[*gwv1.Gateway, gwv1.GatewayStatus],
	krt.Collection[*translator.GatewayListener],
) {
	return translator.GatewayCollection(
		s.controllerName,
		s.agwCollections.Gateways,
		listenerSets,
		gatewayClasses,
		s.agwCollections.Namespaces,
		refGrants,
		s.agwCollections.Secrets,
		krtopts,
	)
}

func (s *Syncer) buildListenerSetCollection(
	gatewayClasses krt.Collection[translator.GatewayClass],
	refGrants translator.ReferenceGrants,
	krtopts krtutil.KrtOptions,
) (
	krt.StatusCollection[*gatewayx.XListenerSet, gatewayx.ListenerSetStatus],
	krt.Collection[translator.ListenerSet],
) {
	return translator.ListenerSetCollection(
		s.controllerName,
		s.agwCollections.XListenerSets,
		s.agwCollections.Gateways,
		gatewayClasses,
		s.agwCollections.Namespaces,
		refGrants,
		s.agwCollections.Secrets,
		krtopts,
	)
}

func (s *Syncer) buildAgwResources(
	gateways krt.Collection[*translator.GatewayListener],
	refGrants translator.ReferenceGrants,
	krtopts krtutil.KrtOptions,
) (krt.Collection[agwir.AgwResource], krt.Collection[*translator.RouteAttachment], PolicyStatusCollections) {
	// filter gateway collections to only include gateways which use a built-in gateway class
	// (resources for additional gateway classes should be created by the downstream providing them)
	filteredGateways := krt.NewCollection(gateways, func(ctx krt.HandlerContext, gw *translator.GatewayListener) **translator.GatewayListener {
		if _, isAdditionalClass := s.additionalGatewayClasses[gw.ParentInfo.ParentGatewayClassName]; isAdditionalClass {
			return nil
		}
		return &gw
	}, krtopts.ToOptions("FilteredGateways")...)

	// Build ports and binds
	ports := krtpkg.UnnamedIndex(filteredGateways, func(l *translator.GatewayListener) []string {
		return []string{fmt.Sprint(l.ParentInfo.Port)}
	}).AsCollection(krtopts.ToOptions("PortBindings")...)

	binds := krt.NewManyCollection(ports, func(ctx krt.HandlerContext, object krt.IndexObject[string, *translator.GatewayListener]) []agwir.AgwResource {
		port, _ := strconv.Atoi(object.Key)
		uniq := sets.New[types.NamespacedName]()
		for _, gw := range object.Objects {
			uniq.Insert(types.NamespacedName{
				Namespace: gw.ParentGateway.Namespace,
				Name:      gw.ParentGateway.Name,
			})
		}
		return slices.Map(uniq.UnsortedList(), func(e types.NamespacedName) agwir.AgwResource {
			bind := translator.AgwBind{
				Bind: &api.Bind{
					Key:  object.Key + "/" + e.String(),
					Port: uint32(port), //nolint:gosec // G115: port is always in valid port range
				},
			}
			return translator.ToResourceForGateway(e, bind)
		})
	}, krtopts.ToOptions("Binds")...)
	if s.agwPlugins.AddResourceExtension != nil && s.agwPlugins.AddResourceExtension.Binds != nil {
		binds = krt.JoinCollection([]krt.Collection[agwir.AgwResource]{binds, s.agwPlugins.AddResourceExtension.Binds})
	}

	// Build listeners
	listeners := krt.NewCollection(filteredGateways, func(ctx krt.HandlerContext, obj *translator.GatewayListener) *agwir.AgwResource {
		return s.buildListenerFromGateway(obj)
	}, krtopts.ToOptions("Listeners")...)
	if s.agwPlugins.AddResourceExtension != nil && s.agwPlugins.AddResourceExtension.Listeners != nil {
		listeners = krt.JoinCollection([]krt.Collection[agwir.AgwResource]{listeners, s.agwPlugins.AddResourceExtension.Listeners})
	}

	// Build routes
	routeParents := translator.BuildRouteParents(filteredGateways)
	routeInputs := translator.RouteContextInputs{
		Grants:          refGrants,
		RouteParents:    routeParents,
		ControllerName:  s.controllerName,
		Services:        s.agwCollections.Services,
		Namespaces:      s.agwCollections.Namespaces,
		InferencePools:  s.agwCollections.InferencePools,
		Backends:        s.agwCollections.Backends,
		DirectResponses: s.agwCollections.DirectResponses,
	}

	agwRoutes, routeAttachments := translator.AgwRouteCollection(s.statusCollections, s.agwCollections.HTTPRoutes, s.agwCollections.GRPCRoutes, s.agwCollections.TCPRoutes, s.agwCollections.TLSRoutes, routeInputs, krtopts)
	if s.agwPlugins.AddResourceExtension != nil && s.agwPlugins.AddResourceExtension.Routes != nil {
		agwRoutes = krt.JoinCollection([]krt.Collection[agwir.AgwResource]{agwRoutes, s.agwPlugins.AddResourceExtension.Routes})
	}

	agwPolicies, policyStatuses := AgwPolicyCollection(s.agwPlugins, krtopts)

	// Create an agentgateway backend collection from the kgateway backend resources
	_, agwBackends := s.newAgwBackendCollection(s.agwCollections.Backends, krtopts)

	// Join all Agw resources
	allAgwResources := krt.JoinCollection([]krt.Collection[agwir.AgwResource]{binds, listeners, agwRoutes, agwPolicies, agwBackends}, krtopts.ToOptions("Resources")...)

	return allAgwResources, routeAttachments, policyStatuses
}

// buildListenerFromGateway creates a listener resource from a gateway
func (s *Syncer) buildListenerFromGateway(obj *translator.GatewayListener) *agwir.AgwResource {
	l := &api.Listener{
		Key:         obj.ResourceName(),
		Name:        string(obj.ParentInfo.SectionName),
		BindKey:     fmt.Sprint(obj.ParentInfo.Port) + "/" + obj.ParentGateway.Namespace + "/" + obj.ParentGateway.Name,
		GatewayName: obj.ParentGateway.Namespace + "/" + obj.ParentGateway.Name,
		Hostname:    obj.ParentInfo.OriginalHostname,
	}

	// Set protocol and TLS configuration
	protocol, tlsConfig, ok := s.getProtocolAndTLSConfig(obj)
	if !ok {
		return nil // Unsupported protocol or missing TLS config
	}

	l.Protocol = protocol
	l.Tls = tlsConfig

	return ptr.Of(translator.ToResourceForGateway(types.NamespacedName{
		Namespace: obj.ParentGateway.Namespace,
		Name:      obj.ParentGateway.Name,
	}, translator.AgwListener{l}))
}

// buildBackendFromBackendIR creates a backend resource from Backend
func (s *Syncer) buildBackendFromBackend(ctx krt.HandlerContext,
	backend *v1alpha1.Backend, svcCol krt.Collection[*corev1.Service],
	secretsCol krt.Collection[*corev1.Secret],
	nsCol krt.Collection[*corev1.Namespace],
) ([]agwir.AgwResource, *v1alpha1.BackendStatus) {
	var results []agwir.AgwResource
	var backendStatus *v1alpha1.BackendStatus
	backends, backendPolicies, err := s.translator.BackendTranslator().TranslateBackend(ctx, backend, svcCol, secretsCol, nsCol)
	if err != nil {
		logger.Error("failed to translate backend", "backend", backend.Name, "namespace", backend.Namespace, "error", err)
		backendStatus = &v1alpha1.BackendStatus{
			Conditions: []metav1.Condition{
				{
					Type:               "Accepted",
					Status:             metav1.ConditionFalse,
					Reason:             "TranslationError",
					Message:            fmt.Sprintf("failed to translate backend %v", err),
					ObservedGeneration: backend.Generation,
				},
			},
		}
		return results, backendStatus
	}
	// handle all backends created as an MCP backend may create multiple backends
	for _, backend := range backends {
		logger.Debug("creating backend", "backend", backend.Name)
		resourceWrapper := translator.ToResourceGlobal(&api.Resource{
			Kind: &api.Resource_Backend{
				Backend: backend,
			},
		})
		results = append(results, resourceWrapper)
	}
	for _, policy := range backendPolicies {
		logger.Debug("creating backend policy", "policy", policy.Name)
		resourceWrapper := translator.ToResourceGlobal(&api.Resource{
			Kind: &api.Resource_Policy{
				Policy: policy,
			},
		})
		results = append(results, resourceWrapper)
	}
	backendStatus = &v1alpha1.BackendStatus{
		Conditions: []metav1.Condition{
			{
				Type:               "Accepted",
				Status:             metav1.ConditionTrue,
				Reason:             "Accepted",
				ObservedGeneration: backend.Generation,
			},
		},
	}
	return results, backendStatus
}

// newADPBackendCollection creates the ADP backend collection for agent gateway resources
func (s *Syncer) newAgwBackendCollection(finalBackends krt.Collection[*v1alpha1.Backend], krtopts krtutil.KrtOptions) (
	krt.StatusCollection[*v1alpha1.Backend, v1alpha1.BackendStatus],
	krt.Collection[agwir.AgwResource],
) {
	return krt.NewStatusManyCollection(finalBackends, func(krtctx krt.HandlerContext, backend *v1alpha1.Backend) (
		*v1alpha1.BackendStatus,
		[]agwir.AgwResource,
	) {
		resources, status := s.buildBackendFromBackend(krtctx, backend, s.agwCollections.Services, s.agwCollections.Secrets, s.agwCollections.Namespaces)
		return status, resources
	}, krtopts.ToOptions("Backends")...)
}

// getProtocolAndTLSConfig extracts protocol and TLS configuration from a gateway
func (s *Syncer) getProtocolAndTLSConfig(obj *translator.GatewayListener) (api.Protocol, *api.TLSConfig, bool) {
	var tlsConfig *api.TLSConfig

	// Build TLS config if needed
	if obj.TLSInfo != nil {
		tlsConfig = &api.TLSConfig{
			Cert:       obj.TLSInfo.Cert,
			PrivateKey: obj.TLSInfo.Key,
		}
	}

	switch obj.ParentInfo.Protocol {
	case gwv1.HTTPProtocolType:
		return api.Protocol_HTTP, nil, true
	case gwv1.HTTPSProtocolType:
		if tlsConfig == nil {
			return api.Protocol_HTTPS, nil, false // TLS required but not configured
		}
		return api.Protocol_HTTPS, tlsConfig, true
	case gwv1.TLSProtocolType:
		if tlsConfig == nil {
			if obj.ParentInfo.TLSPassthrough {
				// For passthrough, we don't want TLS config
				return api.Protocol_TLS, nil, true
			} else {
				// TLS required but not configured
				return api.Protocol_TLS, nil, false
			}
		}
		return api.Protocol_TLS, tlsConfig, true
	case gwv1.TCPProtocolType:
		return api.Protocol_TCP, nil, true
	default:
		return api.Protocol_HTTP, nil, false // Unsupported protocol
	}
}

func (s *Syncer) buildAddressCollections(krtopts krtutil.KrtOptions) krt.Collection[Address] {
	// Build workload index
	workloadIndex := index{
		namespaces:      s.agwCollections.Namespaces,
		SystemNamespace: s.agwCollections.SystemNamespace,
		ClusterID:       s.agwCollections.ClusterID,
	}
	waypoints := workloadIndex.WaypointsCollection(s.agwCollections.Gateways, s.agwCollections.GatewayClasses, s.agwCollections.Pods, krtopts)

	// Build service and workload collections
	workloadServices := workloadIndex.ServicesCollection(
		s.agwCollections.Services,
		nil,
		waypoints,
		s.agwCollections.InferencePools,
		s.agwCollections.Namespaces,
		krtopts,
	)
	NodeLocality := NodesCollection(s.agwCollections.Nodes, krtopts.ToOptions("NodeLocality")...)
	workloads := workloadIndex.WorkloadsCollection(
		s.agwCollections.Pods,
		NodeLocality,
		workloadServices,
		s.agwCollections.EndpointSlices,
		krtopts,
	)

	// Build address collections
	workloadAddresses := krt.MapCollection(workloads, func(t WorkloadInfo) Address {
		return Address{Workload: &t}
	})
	svcAddresses := krt.MapCollection(workloadServices, func(t ServiceInfo) Address {
		return Address{Service: &t}
	})

	adpAddresses := krt.JoinCollection([]krt.Collection[Address]{svcAddresses, workloadAddresses}, krtopts.ToOptions("Addresses")...)
	return adpAddresses
}

func (s *Syncer) buildXDSCollection(
	agwResources krt.Collection[agwir.AgwResource],
	xdsAddresses krt.Collection[Address],
	krtopts krtutil.KrtOptions,
) {
	// Create an index on adpResources by Gateway to avoid fetching all resources
	agwResourcesByGateway := func(resource agwir.AgwResource) types.NamespacedName {
		return resource.Gateway
	}
	s.Registrations = append(s.Registrations, krtxds.Collection[Address, *api.Address](xdsAddresses, krtopts))
	s.Registrations = append(s.Registrations, krtxds.PerGatewayCollection[agwir.AgwResource, *api.Resource](agwResources, agwResourcesByGateway, krtopts))
}

func (s *Syncer) setupSyncDependencies(
	agwResources krt.Collection[agwir.AgwResource],
	addresses krt.Collection[Address],
) {
	s.waitForSync = []cache.InformerSynced{
		s.agwCollections.HasSynced,
		s.agwPlugins.HasSynced,
		agwResources.HasSynced,
		addresses.HasSynced,
	}
}

func (s *Syncer) Start(ctx context.Context) error {
	logger.Info("starting agentgateway Syncer", "controllername", s.controllerName)
	logger.Info("waiting for agentgateway cache to sync")

	// wait for krt collections to sync
	logger.Info("waiting for cache to sync")
	s.client.WaitForCacheSync(
		"agent gateway status syncer",
		ctx.Done(),
		s.waitForSync...,
	)

	logger.Info("caches warm!")

	s.ready.Store(true)
	<-ctx.Done()
	return nil
}

func (s *Syncer) HasSynced() bool {
	return s.ready.Load()
}

// NeedLeaderElection returns false to ensure that the Syncer runs on all pods (leader and followers)
func (r *Syncer) NeedLeaderElection() bool {
	return false
}

// WaitForSync returns a list of functions that can be used to determine if all its informers have synced.
// This is useful for determining if caches have synced.
// It must be called only after `Init()`.
func (s *Syncer) CacheSyncs() []cache.InformerSynced {
	return s.waitForSync
}

type agentGwSnapshot struct {
	Resources  envoycache.Resources
	Addresses  envoycache.Resources
	VersionMap map[string]map[string]string
}

func (m *agentGwSnapshot) GetResources(typeURL string) map[string]envoytypes.Resource {
	resources := m.GetResourcesAndTTL(typeURL)
	result := make(map[string]envoytypes.Resource, len(resources))
	for k, v := range resources {
		result[k] = v.Resource
	}
	return result
}

func (m *agentGwSnapshot) GetResourcesAndTTL(typeURL string) map[string]envoytypes.ResourceWithTTL {
	switch typeURL {
	case translator.TargetTypeResourceUrl:
		return m.Resources.Items
	case translator.TargetTypeAddressUrl:
		return m.Addresses.Items
	default:
		return nil
	}
}

func (m *agentGwSnapshot) GetVersion(typeURL string) string {
	switch typeURL {
	case translator.TargetTypeResourceUrl:
		return m.Resources.Version
	case translator.TargetTypeAddressUrl:
		return m.Addresses.Version
	default:
		return ""
	}
}

func (m *agentGwSnapshot) ConstructVersionMap() error {
	if m == nil {
		return fmt.Errorf("missing snapshot")
	}
	if m.VersionMap != nil {
		return nil
	}

	m.VersionMap = make(map[string]map[string]string)
	resources := map[string]map[string]envoytypes.ResourceWithTTL{
		translator.TargetTypeResourceUrl: m.Resources.Items,
		translator.TargetTypeAddressUrl:  m.Addresses.Items,
	}

	for typeUrl, items := range resources {
		inner := make(map[string]string, len(items))
		for _, r := range items {
			marshaled, err := envoycache.MarshalResource(r.Resource)
			if err != nil {
				return err
			}
			v := envoycache.HashResource(marshaled)
			if v == "" {
				return fmt.Errorf("failed to build resource version")
			}
			inner[envoycache.GetResourceName(r.Resource)] = v
		}
		m.VersionMap[typeUrl] = inner
	}
	return nil
}

func (m *agentGwSnapshot) GetVersionMap(typeURL string) map[string]string {
	return m.VersionMap[typeURL]
}

var _ envoycache.ResourceSnapshot = &agentGwSnapshot{}
