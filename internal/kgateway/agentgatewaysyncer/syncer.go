package agentgatewaysyncer

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/agentgateway/agentgateway/go/api"
	envoytypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

var (
	logger                                = logging.New("agentgateway/syncer")
	_      manager.LeaderElectionRunnable = &AgentGwSyncer{}
)

const (
	// Retry configuration constants
	maxRetryAttempts = 5
	retryDelay       = 100 * time.Millisecond

	// Resource name format strings
	resourceNameFormat = "%s~%s"
	bindKeyFormat      = "%s/%s"
	gatewayNameFormat  = "%s/%s"

	// Log message keys
	logKeyControllerName = "controllername"
	logKeyError          = "error"
	logKeyGateway        = "gateway"
	logKeyResourceRef    = "resource_ref"
	logKeyRouteType      = "route_type"
)

// AgentGwSyncer synchronizes Kubernetes Gateway API resources with xDS for agentgateway proxies.
// It watches Gateway resources with the agentgateway class and translates them to agentgateway configuration.
type AgentGwSyncer struct {
	// Core collections and dependencies
	agwCollections *plugins.AgwCollections
	mgr            manager.Manager
	client         kube.Client
	plugins        pluginsdk.Plugin
	policyManager  *plugins.DefaultPolicyManager
	translator     *translator.AgentGatewayTranslator

	// Configuration
	controllerName        string
	agentGatewayClassName string
	systemNamespace       string
	clusterID             string

	// XDS and caching
	xDS      krt.Collection[agentGwXdsResources]
	xdsCache envoycache.SnapshotCache

	// Status reporting
	gatewayReports         krt.Singleton[GatewayReports]
	listenerSetReports     krt.Singleton[ListenerSetReports]
	routeReports           krt.Singleton[RouteReports]
	gatewayReportQueue     utils.AsyncQueue[GatewayReports]
	listenerSetReportQueue utils.AsyncQueue[ListenerSetReports]
	routeReportQueue       utils.AsyncQueue[RouteReports]

	// Synchronization
	waitForSync []cache.InformerSynced
	ready       atomic.Bool

	// features
	EnableInferExt bool
}

func NewAgentGwSyncer(
	controllerName string,
	agentGatewayClassName string,
	client kube.Client,
	mgr manager.Manager,
	agwCollections *plugins.AgwCollections,
	plugins pluginsdk.Plugin,
	policyManager *plugins.DefaultPolicyManager,
	xdsCache envoycache.SnapshotCache,
	systemNamespace string,
	clusterID string,
	enableInferExt bool,
) *AgentGwSyncer {
	return &AgentGwSyncer{
		agwCollections:         agwCollections,
		controllerName:         controllerName,
		agentGatewayClassName:  agentGatewayClassName,
		plugins:                plugins,
		policyManager:          policyManager,
		translator:             translator.NewAgentGatewayTranslator(agwCollections, plugins),
		xdsCache:               xdsCache,
		client:                 client,
		mgr:                    mgr,
		systemNamespace:        systemNamespace,
		clusterID:              clusterID,
		EnableInferExt:         enableInferExt,
		gatewayReportQueue:     utils.NewAsyncQueue[GatewayReports](),
		listenerSetReportQueue: utils.NewAsyncQueue[ListenerSetReports](),
		routeReportQueue:       utils.NewAsyncQueue[RouteReports](),
	}
}

func (s *AgentGwSyncer) Init(krtopts krtinternal.KrtOptions) {
	logger.Debug("init agentgateway Syncer", "controllername", s.controllerName)

	s.translator.Init()

	finalBackends, _ := s.buildBackendCollections(krtopts)

	// Pass finalBackends into buildResourceCollections instead of storing on syncer
	s.buildResourceCollections(finalBackends, krtopts)
}

func (s *AgentGwSyncer) buildResourceCollections(finalBackends krt.Collection[ir.BackendObjectIR], krtopts krtinternal.KrtOptions) {
	// Build core collections for irs
	gatewayClasses := GatewayClassesCollection(s.agwCollections.GatewayClasses, krtopts)
	refGrants := BuildReferenceGrants(ReferenceGrantsCollection(s.agwCollections.ReferenceGrants, krtopts))
	gateways := s.buildGatewayCollection(gatewayClasses, refGrants, krtopts)

	// Build ADP resources for gateway
	adpResources := s.buildADPResources(gateways, refGrants, krtopts)

	// Create ADP backend collection from finalBackends
	adpBackends := s.newADPBackendCollection(finalBackends, krtopts)

	// Build address collections
	addresses := s.buildAddressCollections(krtopts)

	// Build XDS collection
	s.buildXDSCollection(adpResources, adpBackends, addresses, krtopts)

	// Build status reporting
	s.buildStatusReporting()

	// Set up sync dependencies
	s.setupSyncDependencies(gateways, adpResources, adpBackends, addresses)
}

func (s *AgentGwSyncer) buildGatewayCollection(
	gatewayClasses krt.Collection[GatewayClass],
	refGrants ReferenceGrants,
	krtopts krtinternal.KrtOptions,
) krt.Collection[GatewayListener] {
	return GatewayCollection(
		s.agentGatewayClassName,
		s.agwCollections.Gateways,
		gatewayClasses,
		s.agwCollections.Namespaces,
		refGrants,
		s.agwCollections.Secrets,
		krtopts,
	)
}

func (s *AgentGwSyncer) buildADPResources(
	gateways krt.Collection[GatewayListener],
	refGrants ReferenceGrants,
	krtopts krtinternal.KrtOptions,
) krt.Collection[ADPResourcesForGateway] {
	// Build ports and binds
	ports := krtpkg.UnnamedIndex(gateways, func(l GatewayListener) []string {
		return []string{fmt.Sprint(l.parentInfo.Port)}
	}).AsCollection(krtopts.ToOptions("PortBindings")...)

	binds := krt.NewManyCollection(ports, func(ctx krt.HandlerContext, object krt.IndexObject[string, GatewayListener]) []ADPResourcesForGateway {
		port, _ := strconv.Atoi(object.Key)
		gwReports := make(map[types.NamespacedName]reports.ReportMap, 0)
		for _, gw := range object.Objects {
			key := types.NamespacedName{
				Namespace: gw.parent.Namespace,
				Name:      gw.parent.Name,
			}
			gwReports[key] = gw.report
		}
		var results []ADPResourcesForGateway
		binds := make(map[types.NamespacedName][]*api.Resource)
		for nsName := range gwReports {
			bind := ADPBind{
				Bind: &api.Bind{
					Key:  object.Key + "/" + nsName.String(),
					Port: uint32(port),
				},
			}
			if binds[nsName] == nil {
				binds[nsName] = make([]*api.Resource, 0)
			}
			binds[nsName] = append(binds[nsName], toADPResource(bind))
		}
		for gw, res := range binds {
			repForGw := gwReports[gw]
			results = append(results, toResourceWithRoutes(gw, res, nil, repForGw))
		}
		return results
	}, krtopts.ToOptions("Binds")...)

	// Build listeners
	listeners := krt.NewCollection(gateways, func(ctx krt.HandlerContext, obj GatewayListener) *ADPResourcesForGateway {
		return s.buildListenerFromGateway(obj)
	}, krtopts.ToOptions("Listeners")...)

	// Build routes
	routeParents := BuildRouteParents(gateways)
	routeInputs := RouteContextInputs{
		Grants:          refGrants,
		RouteParents:    routeParents,
		Services:        s.agwCollections.Services,
		Namespaces:      s.agwCollections.Namespaces,
		InferencePools:  s.agwCollections.InferencePools,
		Backends:        s.agwCollections.BackendIndex,
		Plugins:         s.plugins,
		DirectResponses: s.agwCollections.DirectResponses,
	}
	adpRoutes := ADPRouteCollection(s.agwCollections.HTTPRoutes, s.agwCollections.GRPCRoutes, s.agwCollections.TCPRoutes, s.agwCollections.TLSRoutes, routeInputs, krtopts, s.plugins)

	adpPolicies := ADPPolicyCollection(s.agwCollections, binds, krtopts, s.policyManager)

	// Join all ADP resources
	allADPResources := krt.JoinCollection([]krt.Collection[ADPResourcesForGateway]{binds, listeners, adpRoutes, adpPolicies}, krtopts.ToOptions("ADPResources")...)

	return allADPResources
}

// buildListenerFromGateway creates a listener resource from a gateway
func (s *AgentGwSyncer) buildListenerFromGateway(obj GatewayListener) *ADPResourcesForGateway {
	l := &api.Listener{
		Key:         obj.ResourceName(),
		Name:        string(obj.parentInfo.SectionName),
		BindKey:     fmt.Sprint(obj.parentInfo.Port) + "/" + obj.parent.Namespace + "/" + obj.parent.Name,
		GatewayName: obj.parent.Namespace + "/" + obj.parent.Name,
		Hostname:    obj.parentInfo.OriginalHostname,
	}

	// Set protocol and TLS configuration
	protocol, tlsConfig, ok := s.getProtocolAndTLSConfig(obj)
	if !ok {
		return nil // Unsupported protocol or missing TLS config
	}

	l.Protocol = protocol
	l.Tls = tlsConfig

	resources := []*api.Resource{toADPResource(ADPListener{l})}
	return toResourcep(types.NamespacedName{
		Namespace: obj.parent.Namespace,
		Name:      obj.parent.Name,
	}, resources, obj.report)
}

// buildBackendFromBackendIR creates a backend resource from BackendObjectIR
func (s *AgentGwSyncer) buildBackendFromBackendIR(ctx krt.HandlerContext, backendIR *ir.BackendObjectIR, svcCol krt.Collection[*corev1.Service], secretsCol krt.Collection[*corev1.Secret], nsCol krt.Collection[*corev1.Namespace]) []envoyResourceWithCustomName {
	var results []envoyResourceWithCustomName
	backends, backendPolicies, err := s.translator.BackendTranslator().TranslateBackend(ctx, backendIR, svcCol, secretsCol, nsCol)
	if err != nil {
		logger.Error("failed to translate backend", "backend", backendIR.Name, "namespace", backendIR.Namespace, "error", err)
		return results
	}
	// handle all backends created as an MCP backend may create multiple backends
	for _, backend := range backends {
		logger.Debug("creating backend", "backend", backend.Name)
		resourceWrapper := &api.Resource{
			Kind: &api.Resource_Backend{
				Backend: backend,
			},
		}
		results = append(results, envoyResourceWithCustomName{
			Message: resourceWrapper,
			Name:    backend.Name,
			version: utils.HashProto(resourceWrapper),
		})
	}
	for _, policy := range backendPolicies {
		logger.Debug("creating backend policy", "policy", policy.Name)
		resourceWrapper := &api.Resource{
			Kind: &api.Resource_Policy{
				Policy: policy,
			},
		}
		results = append(results, envoyResourceWithCustomName{
			Message: resourceWrapper,
			Name:    policy.Name,
			version: utils.HashProto(resourceWrapper),
		})
	}
	return results
}

// newADPBackendCollection creates the ADP backend collection for agent gateway resources
func (s *AgentGwSyncer) newADPBackendCollection(finalBackends krt.Collection[ir.BackendObjectIR], krtopts krtinternal.KrtOptions) krt.Collection[envoyResourceWithCustomName] {
	backends := krt.NewManyCollection(finalBackends, func(ctx krt.HandlerContext, backendIR ir.BackendObjectIR) []envoyResourceWithCustomName {
		if backendIR.Group == wellknown.ServiceGVK.Group && backendIR.Kind == wellknown.ServiceGVK.Kind {
			return nil
		}
		return s.buildBackendFromBackendIR(ctx, &backendIR, s.agwCollections.Services, s.agwCollections.Secrets, s.agwCollections.Namespaces)
	}, krtopts.ToOptions("ADPBackends")...)

	return backends
}

// buildBackendCollections builds the filtered backend IR collection and the corresponding ADP backend collection
func (s *AgentGwSyncer) buildBackendCollections(
	krtopts krtinternal.KrtOptions,
) (krt.Collection[ir.BackendObjectIR], krt.Collection[envoyResourceWithCustomName]) {
	// Get all backends with attached policies, filtering out Service backends
	// Agent gateway handles Service references directly in routes and doesn't need separate backend objects
	allBackends := krt.JoinCollection(s.agwCollections.BackendIndex.BackendsWithPolicy(),
		append(krtopts.ToOptions("AllBackends"), krt.WithJoinUnchecked())...)

	finalBackends := krt.NewCollection(allBackends, func(kctx krt.HandlerContext, backend *ir.BackendObjectIR) *ir.BackendObjectIR {
		if backend.Group == wellknown.ServiceGVK.Group && backend.Kind == wellknown.ServiceGVK.Kind {
			return nil
		}
		return backend
	}, krtopts.ToOptions("FinalBackends")...)

	adpBackends := s.newADPBackendCollection(finalBackends, krtopts)
	return finalBackends, adpBackends
}

// getProtocolAndTLSConfig extracts protocol and TLS configuration from a gateway
func (s *AgentGwSyncer) getProtocolAndTLSConfig(obj GatewayListener) (api.Protocol, *api.TLSConfig, bool) {
	var tlsConfig *api.TLSConfig

	// Build TLS config if needed
	if obj.TLSInfo != nil {
		tlsConfig = &api.TLSConfig{
			Cert:       obj.TLSInfo.Cert,
			PrivateKey: obj.TLSInfo.Key,
		}
	}

	switch obj.parentInfo.Protocol {
	case gwv1.HTTPProtocolType:
		return api.Protocol_HTTP, nil, true
	case gwv1.HTTPSProtocolType:
		if tlsConfig == nil {
			return api.Protocol_HTTPS, nil, false // TLS required but not configured
		}
		return api.Protocol_HTTPS, tlsConfig, true
	case gwv1.TLSProtocolType:
		if tlsConfig == nil {
			return api.Protocol_TLS, nil, false // TLS required but not configured
		}
		return api.Protocol_TLS, tlsConfig, true
	case gwv1.TCPProtocolType:
		return api.Protocol_TCP, nil, true
	default:
		return api.Protocol_HTTP, nil, false // Unsupported protocol
	}
}

func (s *AgentGwSyncer) buildAddressCollections(krtopts krtinternal.KrtOptions) krt.Collection[envoyResourceWithCustomName] {
	// Build workload index
	workloadIndex := index{
		namespaces:      s.agwCollections.Namespaces,
		SystemNamespace: s.systemNamespace,
		ClusterID:       s.clusterID,
	}

	// Build service and workload collections
	workloadServices := workloadIndex.ServicesCollection(s.agwCollections.Services, nil, s.agwCollections.InferencePools, s.agwCollections.Namespaces, krtopts)
	workloads := workloadIndex.WorkloadsCollection(
		s.agwCollections.WrappedPods,
		workloadServices,
		s.agwCollections.EndpointSlices,
		krtopts,
	)

	// Build address collections
	svcAddresses := krt.NewCollection(workloadServices, func(ctx krt.HandlerContext, obj ServiceInfo) *ADPCacheAddress {
		addrMessage := obj.AsAddress.Address
		resourceVersion := utils.HashProto(addrMessage)
		result := &ADPCacheAddress{
			NamespacedName:      types.NamespacedName{Name: obj.Service.GetName(), Namespace: obj.Service.GetNamespace()},
			Address:             addrMessage,
			AddressResourceName: obj.ResourceName(),
			AddressVersion:      resourceVersion,
		}
		logger.Debug("created XDS resources for svc address with ID", "addr", fmt.Sprintf("%s,%s", obj.Service.GetName(), obj.Service.GetNamespace()), "resourceid", result.ResourceName())
		return result
	})

	workloadAddresses := krt.NewCollection(workloads, func(ctx krt.HandlerContext, obj WorkloadInfo) *ADPCacheAddress {
		addrMessage := obj.AsAddress.Address
		resourceVersion := utils.HashProto(addrMessage)
		result := &ADPCacheAddress{
			NamespacedName:      types.NamespacedName{Name: obj.Workload.GetName(), Namespace: obj.Workload.GetNamespace()},
			Address:             addrMessage,
			AddressVersion:      resourceVersion,
			AddressResourceName: obj.ResourceName(),
		}
		logger.Debug("created XDS resources for workload address with ID", "addr", fmt.Sprintf("%s,%s", obj.Workload.GetName(), obj.Workload.GetNamespace()), "resourceid", result.ResourceName())
		return result
	})

	adpAddresses := krt.JoinCollection([]krt.Collection[ADPCacheAddress]{svcAddresses, workloadAddresses}, krtopts.ToOptions("ADPAddresses")...)
	return krt.NewCollection(adpAddresses, func(kctx krt.HandlerContext, obj ADPCacheAddress) *envoyResourceWithCustomName {
		return &envoyResourceWithCustomName{
			Message: obj.Address,
			Name:    obj.AddressResourceName,
			version: obj.AddressVersion,
		}
	}, krtopts.ToOptions("XDSAddresses")...)
}

func (s *AgentGwSyncer) buildXDSCollection(
	adpResources krt.Collection[ADPResourcesForGateway],
	adpBackends krt.Collection[envoyResourceWithCustomName],
	xdsAddresses krt.Collection[envoyResourceWithCustomName],
	krtopts krtinternal.KrtOptions,
) {
	// Create an index on adpResources by Gateway to avoid fetching all resources
	adpResourcesByGateway := krt.NewIndex(adpResources, "gateway", func(resource ADPResourcesForGateway) []types.NamespacedName {
		return []types.NamespacedName{resource.Gateway}
	})

	s.xDS = krt.NewCollection(adpResources, func(kctx krt.HandlerContext, obj ADPResourcesForGateway) *agentGwXdsResources {
		gwNamespacedName := obj.Gateway

		cacheAddresses := krt.Fetch(kctx, xdsAddresses)
		envoytypesAddresses := make([]envoytypes.Resource, 0, len(cacheAddresses))
		for _, addr := range cacheAddresses {
			envoytypesAddresses = append(envoytypesAddresses, &addr)
		}

		// Create a copy of the shared ReportMap to avoid concurrent modification
		gwReports := reports.NewReportMap()

		var cacheResources []envoytypes.Resource
		attachedRoutes := make(map[string]uint)
		// Use index to fetch only resources for this gateway instead of all resources
		resourceList := krt.Fetch(kctx, adpResources, krt.FilterIndex(adpResourcesByGateway, gwNamespacedName))
		for _, resource := range resourceList {
			// 1. merge GW Reports for all Proxies' status reports
			maps.Copy(gwReports.Gateways, resource.report.Gateways)

			// 2. merge LS Reports for all Proxies' status reports
			maps.Copy(gwReports.ListenerSets, resource.report.ListenerSets)

			// 3. merge route parentRefs into RouteReports for all route types
			mergeRouteReports(gwReports.HTTPRoutes, resource.report.HTTPRoutes)
			mergeRouteReports(gwReports.TCPRoutes, resource.report.TCPRoutes)
			mergeRouteReports(gwReports.TLSRoutes, resource.report.TLSRoutes)
			mergeRouteReports(gwReports.GRPCRoutes, resource.report.GRPCRoutes)

			for key, rr := range resource.report.Policies {
				// if we haven't encountered this policy, just copy it over completely
				old := gwReports.Policies[key]
				if old == nil {
					gwReports.Policies[key] = rr
					continue
				}
				// else, let's merge our parentRefs into the existing map
				// obsGen will stay as-is...
				maps.Copy(gwReports.Policies[key].Ancestors, rr.Ancestors)
			}

			for _, res := range resource.Resources {
				cacheResources = append(cacheResources, &envoyResourceWithCustomName{
					Message: res,
					Name:    getADPResourceName(res),
					version: utils.HashProto(res),
				})
				for listenerName, count := range resource.attachedRoutes {
					attachedRoutes[listenerName] += count
				}
			}
		}

		// Fetch all backends and add them to the resources for every gateway
		cachedBackends := krt.Fetch(kctx, adpBackends)
		for _, backend := range cachedBackends {
			cacheResources = append(cacheResources, &backend)
		}

		// Create the resource wrappers
		var resourceVersion uint64
		for _, res := range cacheResources {
			resourceVersion ^= res.(*envoyResourceWithCustomName).version
		}
		// Calculate address version
		var addrVersion uint64
		for _, res := range cacheAddresses {
			addrVersion ^= res.version
		}

		result := &agentGwXdsResources{
			NamespacedName: gwNamespacedName,
			reports:        gwReports,
			attachedRoutes: attachedRoutes,
			ResourceConfig: envoycache.NewResources(fmt.Sprintf("%d", resourceVersion), cacheResources),
			AddressConfig:  envoycache.NewResources(fmt.Sprintf("%d", addrVersion), envoytypesAddresses),
		}
		logger.Debug("created XDS resources for gateway with ID", "gwname", fmt.Sprintf("%s,%s", gwNamespacedName.Name, gwNamespacedName.Namespace), "resourceid", result.ResourceName())
		return result
	}, krtopts.ToOptions("agent-xds")...)
}

func (s *AgentGwSyncer) buildStatusReporting() {
	// TODO(npolshak): Move away from report map and separately fetch resource reports
	// Create separate singleton collections for each resource type instead of merging everything
	// This avoids the overhead of creating and processing a single large merged report
	gatewayReports := krt.NewSingleton(func(kctx krt.HandlerContext) *GatewayReports {
		proxies := krt.Fetch(kctx, s.xDS)
		merged := make(map[types.NamespacedName]*reports.GatewayReport)
		attachedRoutes := make(map[types.NamespacedName]map[string]uint)

		for _, p := range proxies {
			// Merge GW Reports for all Proxies' status reports
			maps.Copy(merged, p.reports.Gateways)

			// Collect attached routes for each gateway
			if attachedRoutes[p.NamespacedName] == nil {
				attachedRoutes[p.NamespacedName] = make(map[string]uint)
			}
			for listener, counts := range p.attachedRoutes {
				attachedRoutes[p.NamespacedName][listener] += counts
			}
		}

		return &GatewayReports{
			Reports:        merged,
			AttachedRoutes: attachedRoutes,
		}
	})

	listenerSetReports := krt.NewSingleton(func(kctx krt.HandlerContext) *ListenerSetReports {
		proxies := krt.Fetch(kctx, s.xDS)
		merged := make(map[types.NamespacedName]*reports.ListenerSetReport)

		for _, p := range proxies {
			// Merge LS Reports for all Proxies' status reports
			maps.Copy(merged, p.reports.ListenerSets)
		}

		return &ListenerSetReports{
			Reports: merged,
		}
	})

	routeReports := krt.NewSingleton(func(kctx krt.HandlerContext) *RouteReports {
		proxies := krt.Fetch(kctx, s.xDS)
		merged := RouteReports{
			HTTPRoutes: make(map[types.NamespacedName]*reports.RouteReport),
			GRPCRoutes: make(map[types.NamespacedName]*reports.RouteReport),
			TCPRoutes:  make(map[types.NamespacedName]*reports.RouteReport),
			TLSRoutes:  make(map[types.NamespacedName]*reports.RouteReport),
		}

		for _, p := range proxies {
			// Merge route parentRefs into RouteReports for all route types
			mergeRouteReports(merged.HTTPRoutes, p.reports.HTTPRoutes)
			mergeRouteReports(merged.GRPCRoutes, p.reports.GRPCRoutes)
			mergeRouteReports(merged.TCPRoutes, p.reports.TCPRoutes)
			mergeRouteReports(merged.TLSRoutes, p.reports.TLSRoutes)
		}

		return &merged
	})

	// Store references to the separate collections
	s.gatewayReports = gatewayReports
	s.listenerSetReports = listenerSetReports
	s.routeReports = routeReports
}

func (s *AgentGwSyncer) setupSyncDependencies(gateways krt.Collection[GatewayListener], adpResources krt.Collection[ADPResourcesForGateway], adpBackends krt.Collection[envoyResourceWithCustomName], addresses krt.Collection[envoyResourceWithCustomName]) {
	s.waitForSync = []cache.InformerSynced{
		s.agwCollections.HasSynced,
		gateways.HasSynced,
		// resources
		adpResources.HasSynced,
		adpBackends.HasSynced,
		s.xDS.HasSynced,
		// addresses
		addresses.HasSynced,
		s.agwCollections.Namespaces.HasSynced,
	}
}

func (s *AgentGwSyncer) Start(ctx context.Context) error {
	logger.Info("starting agentgateway Syncer", "controllername", s.controllerName)
	logger.Info("waiting for agentgateway cache to sync")

	// wait for krt collections to sync
	logger.Info("waiting for cache to sync")
	s.client.WaitForCacheSync(
		"agent gateway status syncer",
		ctx.Done(),
		s.waitForSync...,
	)

	// wait for ctrl-rtime caches to sync before accepting events
	if !s.mgr.GetCache().WaitForCacheSync(ctx) {
		return fmt.Errorf("agent gateway sync loop waiting for all caches to sync failed")
	}
	logger.Info("caches warm!")

	// Register to separate singleton collections instead of a single merged report
	s.gatewayReports.Register(func(o krt.Event[GatewayReports]) {
		if o.Event == controllers.EventDelete {
			// TODO: handle garbage collection
			return
		}
		s.gatewayReportQueue.Enqueue(o.Latest())
	})

	s.listenerSetReports.Register(func(o krt.Event[ListenerSetReports]) {
		if o.Event == controllers.EventDelete {
			// TODO: handle garbage collection
			return
		}
		s.listenerSetReportQueue.Enqueue(o.Latest())
	})

	s.routeReports.Register(func(o krt.Event[RouteReports]) {
		if o.Event == controllers.EventDelete {
			// TODO: handle garbage collection
			return
		}
		s.routeReportQueue.Enqueue(o.Latest())
	})

	s.xDS.RegisterBatch(func(events []krt.Event[agentGwXdsResources]) {
		for _, e := range events {
			snap := e.Latest()
			if e.Event == controllers.EventDelete {
				// TODO: we should probably clear, but this has been causing some undiagnosed issues.
				//s.xdsCache.ClearSnapshot(snap.ResourceName())
				continue
			}
			snapshot := &agentGwSnapshot{
				Resources: snap.ResourceConfig,
				Addresses: snap.AddressConfig,
			}
			logger.Debug("setting xds snapshot", "resource_name", snap.ResourceName())
			logger.Debug("snapshot config", "resource_snapshot", snapshot.Resources, "workload_snapshot", snapshot.Addresses)
			err := s.xdsCache.SetSnapshot(ctx, snap.ResourceName(), snapshot)
			if err != nil {
				logger.Error("failed to set xds snapshot", "resource_name", snap.ResourceName(), "error", err.Error())
				continue
			}
		}
	}, true)

	s.ready.Store(true)
	<-ctx.Done()
	return nil
}

func (s *AgentGwSyncer) HasSynced() bool {
	return s.ready.Load()
}

// NeedLeaderElection returns false to ensure that the AgentGwSyncer runs on all pods (leader and followers)
func (r *AgentGwSyncer) NeedLeaderElection() bool {
	return false
}

// ReportQueue returns the queue that contains the latest GatewayReports.
// It will be constantly updated to contain the merged status report for Kube Gateway status.
func (s *AgentGwSyncer) GatewayReportQueue() utils.AsyncQueue[GatewayReports] {
	return s.gatewayReportQueue
}

// ListenerSetReportQueue returns the queue that contains the latest ListenerSetReports.
// It will be constantly updated to contain the merged status report for Kube Gateway status.
func (s *AgentGwSyncer) ListenerSetReportQueue() utils.AsyncQueue[ListenerSetReports] {
	return s.listenerSetReportQueue
}

// RouteReportQueue returns the queue that contains the latest RouteReports.
// It will be constantly updated to contain the merged status report for Kube Gateway status.
func (s *AgentGwSyncer) RouteReportQueue() utils.AsyncQueue[RouteReports] {
	return s.routeReportQueue
}

// WaitForSync returns a list of functions that can be used to determine if all its informers have synced.
// This is useful for determining if caches have synced.
// It must be called only after `Init()`.
func (s *AgentGwSyncer) CacheSyncs() []cache.InformerSynced {
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
	case TargetTypeResourceUrl:
		return m.Resources.Items
	case TargetTypeAddressUrl:
		return m.Addresses.Items
	default:
		return nil
	}
}

func (m *agentGwSnapshot) GetVersion(typeURL string) string {
	switch typeURL {
	case TargetTypeResourceUrl:
		return m.Resources.Version
	case TargetTypeAddressUrl:
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
		TargetTypeResourceUrl: m.Resources.Items,
		TargetTypeAddressUrl:  m.Addresses.Items,
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

// TODO: refactor proxy_syncer status syncing to use the same logic as agentgateway syncer

// mergeRouteReports is a helper function to merge route reports
func mergeRouteReports(merged map[types.NamespacedName]*reports.RouteReport, source map[types.NamespacedName]*reports.RouteReport) {
	for rnn, rr := range source {
		// if we haven't encountered this route, just copy it over completely
		old := merged[rnn]
		if old == nil {
			merged[rnn] = rr
			continue
		}
		// else, this route has already been seen for a proxy, merge this proxy's parents
		// into the merged report
		maps.Copy(merged[rnn].Parents, rr.Parents)
	}
}
