package agentgatewaysyncer

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"sync/atomic"

	"github.com/agentgateway/agentgateway/go/api"
	envoytypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"istio.io/istio/pilot/pkg/status"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	agwir "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

var (
	logger                                = logging.New("agentgateway/syncer")
	_      manager.LeaderElectionRunnable = &Syncer{}
)

const (

	// Log message keys
	logKeyControllerName = "controllername"
	logKeyGateway        = "gateway"
	logKeyResourceRef    = "resource_ref"
	logKeyRouteType      = "route_type"
)

// Syncer synchronizes Kubernetes Gateway API resources with xDS for agentgateway proxies.
// It watches Gateway resources with the agentgateway class and translates them to agentgateway configuration.
type Syncer struct {
	// Core collections and dependencies
	agwCollections *plugins.AgwCollections
	mgr            manager.Manager
	client         kube.Client
	agwPlugins     plugins.AgwPlugin
	translator     *translator.AgwTranslator

	// Configuration
	controllerName        string
	agentgatewayClassName string

	// XDS and caching
	xDS      krt.Collection[translator.AgentGwXdsResources]
	xdsCache envoycache.SnapshotCache

	// Status reporting
	gatewayReports         krt.Singleton[translator.GatewayReports]
	listenerSetReports     krt.Singleton[translator.ListenerSetReports]
	routeReports           krt.Singleton[translator.RouteReports]
	gatewayReportQueue     utils.AsyncQueue[translator.GatewayReports]
	listenerSetReportQueue utils.AsyncQueue[translator.ListenerSetReports]
	routeReportQueue       utils.AsyncQueue[translator.RouteReports]
	policyStatusQueue      *status.StatusCollections

	// Collection status reporting
	// TODO(npolshak): report these separately from proxy_syncer backends https://github.com/kgateway-dev/kgateway/issues/11966
	//backendStatuses krt.StatusCollection[*v1alpha1.Backend, v1alpha1.BackendStatus]

	// Synchronization
	waitForSync []cache.InformerSynced
	ready       atomic.Bool

	// features
	EnableInferExt bool
}

func NewAgwSyncer(
	controllerName string,
	agentgatewayClassName string,
	client kube.Client,
	mgr manager.Manager,
	agwCollections *plugins.AgwCollections,
	agwPlugins plugins.AgwPlugin,
	xdsCache envoycache.SnapshotCache,
	enableInferExt bool,
) *Syncer {
	return &Syncer{
		agwCollections:         agwCollections,
		controllerName:         controllerName,
		agentgatewayClassName:  agentgatewayClassName,
		agwPlugins:             agwPlugins,
		translator:             translator.NewAgwTranslator(agwCollections),
		xdsCache:               xdsCache,
		client:                 client,
		mgr:                    mgr,
		EnableInferExt:         enableInferExt,
		gatewayReportQueue:     utils.NewAsyncQueue[translator.GatewayReports](),
		listenerSetReportQueue: utils.NewAsyncQueue[translator.ListenerSetReports](),
		routeReportQueue:       utils.NewAsyncQueue[translator.RouteReports](),
		policyStatusQueue:      &status.StatusCollections{},
	}
}

// PolicyStatusAsyncQueue wraps AsyncQueue to implement controllers.Writer interface for Istio's StatusCollections
// See: https://github.com/istio/istio/blob/531c61709aaa9bc9187c625e9e460be98f2abf2e/pilot/pkg/status/manager.go#L107
type PolicyStatusAsyncQueue struct {
	queue utils.AsyncQueue[krt.ObjectWithStatus[controllers.Object, gwv1alpha2.PolicyStatus]]
}

func (b *PolicyStatusAsyncQueue) Enqueue(obj krt.ObjectWithStatus[controllers.Object, gwv1alpha2.PolicyStatus]) {
	b.queue.Enqueue(obj)
}

// GetAsyncQueue returns the underlying AsyncQueue for use in status syncer
func (b *PolicyStatusAsyncQueue) GetAsyncQueue() utils.AsyncQueue[krt.ObjectWithStatus[controllers.Object, gwv1alpha2.PolicyStatus]] {
	return b.queue
}

// NewPolicyStatusAsyncQueue creates a new PolicyStatusAsyncQueue
func NewPolicyStatusAsyncQueue() *PolicyStatusAsyncQueue {
	return &PolicyStatusAsyncQueue{
		queue: utils.NewAsyncQueue[krt.ObjectWithStatus[controllers.Object, gwv1alpha2.PolicyStatus]](),
	}
}

func (s *Syncer) Init(krtopts krtutil.KrtOptions) {
	logger.Debug("init agentgateway Syncer", "controllername", s.controllerName)

	s.translator.Init()
	s.buildResourceCollections(krtopts)
}

func (s *Syncer) PolicyStatusQueue() *status.StatusCollections {
	return s.policyStatusQueue
}

func (s *Syncer) buildResourceCollections(krtopts krtutil.KrtOptions) {
	// Build core collections for irs
	gatewayClasses := translator.GatewayClassesCollection(s.agwCollections.GatewayClasses, krtopts)
	refGrants := translator.BuildReferenceGrants(translator.ReferenceGrantsCollection(s.agwCollections.ReferenceGrants, krtopts))
	gateways := s.buildGatewayCollection(gatewayClasses, refGrants, krtopts)

	// Build Agw resources for gateway
	agwResources, policyStatuses := s.buildAgwResources(gateways, refGrants, krtopts)

	// Create an agentgateway backend collection from the kgateway backend resources
	_, agwBackends := s.newAgwBackendCollection(s.agwCollections.Backends, krtopts)

	// Build address collections
	addresses := s.buildAddressCollections(krtopts)

	// Build XDS collection
	s.buildXDSCollection(agwResources, agwBackends, addresses, krtopts)

	// Build status reporting
	s.buildStatusReporting(policyStatuses)

	// Set up sync dependencies
	s.setupSyncDependencies(gateways, agwResources, agwBackends, addresses)
}

func (s *Syncer) buildGatewayCollection(
	gatewayClasses krt.Collection[translator.GatewayClass],
	refGrants translator.ReferenceGrants,
	krtopts krtutil.KrtOptions,
) krt.Collection[translator.GatewayListener] {
	return translator.GatewayCollection(
		s.controllerName,
		s.agwCollections.Gateways,
		gatewayClasses,
		s.agwCollections.Namespaces,
		refGrants,
		s.agwCollections.Secrets,
		krtopts,
	)
}

func (s *Syncer) buildAgwResources(
	gateways krt.Collection[translator.GatewayListener],
	refGrants translator.ReferenceGrants,
	krtopts krtutil.KrtOptions,
) (krt.Collection[agwir.AgwResourcesForGateway], map[schema.GroupKind]krt.StatusCollection[controllers.Object, gwv1alpha2.PolicyStatus]) {
	// Build ports and binds
	ports := krtpkg.UnnamedIndex(gateways, func(l translator.GatewayListener) []string {
		return []string{fmt.Sprint(l.ParentInfo.Port)}
	}).AsCollection(krtopts.ToOptions("PortBindings")...)

	binds := krt.NewManyCollection(ports, func(ctx krt.HandlerContext, object krt.IndexObject[string, translator.GatewayListener]) []agwir.AgwResourcesForGateway {
		port, _ := strconv.Atoi(object.Key)
		gwReports := make(map[types.NamespacedName]reports.ReportMap, 0)
		for _, gw := range object.Objects {
			key := types.NamespacedName{
				Namespace: gw.Parent.Namespace,
				Name:      gw.Parent.Name,
			}
			gwReports[key] = gw.Report
		}
		var results []agwir.AgwResourcesForGateway
		binds := make(map[types.NamespacedName][]*api.Resource)
		for nsName := range gwReports {
			bind := translator.AgwBind{
				Bind: &api.Bind{
					Key:  object.Key + "/" + nsName.String(),
					Port: uint32(port), //nolint:gosec // G115: port is always in valid port range
				},
			}
			if binds[nsName] == nil {
				binds[nsName] = make([]*api.Resource, 0)
			}
			binds[nsName] = append(binds[nsName], translator.ToAgwResource(bind))
		}
		for gw, res := range binds {
			repForGw := gwReports[gw]
			results = append(results, translator.ToResourceWithRoutes(gw, res, nil, repForGw))
		}
		return results
	}, krtopts.ToOptions("Binds")...)
	if s.agwPlugins.AddResourceExtension != nil && s.agwPlugins.AddResourceExtension.Binds != nil {
		binds = krt.JoinCollection([]krt.Collection[agwir.AgwResourcesForGateway]{binds, s.agwPlugins.AddResourceExtension.Binds})
	}

	// Build listeners
	listeners := krt.NewCollection(gateways, func(ctx krt.HandlerContext, obj translator.GatewayListener) *agwir.AgwResourcesForGateway {
		return s.buildListenerFromGateway(obj)
	}, krtopts.ToOptions("Listeners")...)
	if s.agwPlugins.AddResourceExtension != nil && s.agwPlugins.AddResourceExtension.Listeners != nil {
		listeners = krt.JoinCollection([]krt.Collection[agwir.AgwResourcesForGateway]{listeners, s.agwPlugins.AddResourceExtension.Listeners})
	}

	// Build routes
	routeParents := translator.BuildRouteParents(gateways)
	routeInputs := translator.RouteContextInputs{
		Grants:          refGrants,
		RouteParents:    routeParents,
		Services:        s.agwCollections.Services,
		Namespaces:      s.agwCollections.Namespaces,
		InferencePools:  s.agwCollections.InferencePools,
		Backends:        s.agwCollections.Backends,
		DirectResponses: s.agwCollections.DirectResponses,
	}
	agwRoutes := translator.AgwRouteCollection(s.agwCollections.HTTPRoutes, s.agwCollections.GRPCRoutes, s.agwCollections.TCPRoutes, s.agwCollections.TLSRoutes, routeInputs, krtopts)
	if s.agwPlugins.AddResourceExtension != nil && s.agwPlugins.AddResourceExtension.Routes != nil {
		agwRoutes = krt.JoinCollection([]krt.Collection[agwir.AgwResourcesForGateway]{agwRoutes, s.agwPlugins.AddResourceExtension.Routes})
	}

	agwPolicies, policyStatuses := AgwPolicyCollection(binds, s.agwPlugins)

	// Join all Agw resources
	allAgwResources := krt.JoinCollection([]krt.Collection[agwir.AgwResourcesForGateway]{binds, listeners, agwRoutes, agwPolicies}, krtopts.ToOptions("AgwResources")...)

	return allAgwResources, policyStatuses
}

// buildListenerFromGateway creates a listener resource from a gateway
func (s *Syncer) buildListenerFromGateway(obj translator.GatewayListener) *agwir.AgwResourcesForGateway {
	l := &api.Listener{
		Key:         obj.ResourceName(),
		Name:        string(obj.ParentInfo.SectionName),
		BindKey:     fmt.Sprint(obj.ParentInfo.Port) + "/" + obj.Parent.Namespace + "/" + obj.Parent.Name,
		GatewayName: obj.Parent.Namespace + "/" + obj.Parent.Name,
		Hostname:    obj.ParentInfo.OriginalHostname,
	}

	// Set protocol and TLS configuration
	protocol, tlsConfig, ok := s.getProtocolAndTLSConfig(obj)
	if !ok {
		return nil // Unsupported protocol or missing TLS config
	}

	l.Protocol = protocol
	l.Tls = tlsConfig

	resources := []*api.Resource{translator.ToAgwResource(translator.AgwListener{l})}
	return translator.ToResourcep(types.NamespacedName{
		Namespace: obj.Parent.Namespace,
		Name:      obj.Parent.Name,
	}, resources, obj.Report)
}

// buildBackendFromBackendIR creates a backend resource from Backend
func (s *Syncer) buildBackendFromBackend(ctx krt.HandlerContext,
	backend *v1alpha1.Backend, svcCol krt.Collection[*corev1.Service],
	secretsCol krt.Collection[*corev1.Secret],
	nsCol krt.Collection[*corev1.Namespace]) ([]translator.AgwResourceWithCustomName, *v1alpha1.BackendStatus) {
	var results []translator.AgwResourceWithCustomName
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
		resourceWrapper := &api.Resource{
			Kind: &api.Resource_Backend{
				Backend: backend,
			},
		}
		results = append(results, translator.AgwResourceWithCustomName{
			Message: resourceWrapper,
			Name:    backend.Name,
			Version: utils.HashProto(resourceWrapper),
		})
	}
	for _, policy := range backendPolicies {
		logger.Debug("creating backend policy", "policy", policy.Name)
		resourceWrapper := &api.Resource{
			Kind: &api.Resource_Policy{
				Policy: policy,
			},
		}
		results = append(results, translator.AgwResourceWithCustomName{
			Message: resourceWrapper,
			Name:    agwir.GetAgwResourceName(resourceWrapper),
			Version: utils.HashProto(resourceWrapper),
		})
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

// newAgwBackendCollection creates the Agw backend collection for agent gateway resources
func (s *Syncer) newAgwBackendCollection(finalBackends krt.Collection[*v1alpha1.Backend], krtopts krtutil.KrtOptions) (krt.StatusCollection[*v1alpha1.Backend, v1alpha1.BackendStatus], krt.Collection[translator.AgwResourceWithCustomName]) {
	return krt.NewStatusManyCollection(finalBackends, func(krtctx krt.HandlerContext, backend *v1alpha1.Backend) (
		*v1alpha1.BackendStatus,
		[]translator.AgwResourceWithCustomName,
	) {
		resources, status := s.buildBackendFromBackend(krtctx, backend, s.agwCollections.Services, s.agwCollections.Secrets, s.agwCollections.Namespaces)
		return status, resources
	}, krtopts.ToOptions("agwBackends")...)
}

// getProtocolAndTLSConfig extracts protocol and TLS configuration from a gateway
func (s *Syncer) getProtocolAndTLSConfig(obj translator.GatewayListener) (api.Protocol, *api.TLSConfig, bool) {
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
			return api.Protocol_TLS, nil, false // TLS required but not configured
		}
		return api.Protocol_TLS, tlsConfig, true
	case gwv1.TCPProtocolType:
		return api.Protocol_TCP, nil, true
	default:
		return api.Protocol_HTTP, nil, false // Unsupported protocol
	}
}

func (s *Syncer) buildAddressCollections(krtopts krtutil.KrtOptions) krt.Collection[translator.AgwResourceWithCustomName] {
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
	svcAddresses := krt.NewCollection(workloadServices, func(ctx krt.HandlerContext, obj ServiceInfo) *agwir.AgwCacheAddress {
		addrMessage := obj.AsAddress.Address
		resourceVersion := utils.HashProto(addrMessage)
		result := &agwir.AgwCacheAddress{
			NamespacedName:      types.NamespacedName{Name: obj.Service.GetName(), Namespace: obj.Service.GetNamespace()},
			ResourceNames:       obj.ResourceName(),
			Address:             addrMessage,
			AddressResourceName: obj.ResourceName(),
			AddressVersion:      resourceVersion,
		}
		logger.Debug("created XDS resources for svc address with ID", "addr", fmt.Sprintf("%s,%s", obj.Service.GetName(), obj.Service.GetNamespace()), "resourceid", result.ResourceName())
		return result
	})

	workloadAddresses := krt.NewCollection(workloads, func(ctx krt.HandlerContext, obj WorkloadInfo) *agwir.AgwCacheAddress {
		addrMessage := obj.AsAddress.Address
		resourceVersion := utils.HashProto(addrMessage)
		result := &agwir.AgwCacheAddress{
			NamespacedName:      types.NamespacedName{Name: obj.Workload.GetName(), Namespace: obj.Workload.GetNamespace()},
			ResourceNames:       obj.ResourceName(),
			Address:             addrMessage,
			AddressVersion:      resourceVersion,
			AddressResourceName: obj.ResourceName(),
		}
		logger.Debug("created XDS resources for workload address with ID", "addr", fmt.Sprintf("%s,%s", obj.Workload.GetName(), obj.Workload.GetNamespace()), "resourceid", result.ResourceName())
		return result
	})

	agwAddresses := krt.JoinCollection([]krt.Collection[agwir.AgwCacheAddress]{svcAddresses, workloadAddresses}, krtopts.ToOptions("agwAddresses")...)
	return krt.NewCollection(agwAddresses, func(kctx krt.HandlerContext, obj agwir.AgwCacheAddress) *translator.AgwResourceWithCustomName {
		return &translator.AgwResourceWithCustomName{
			Message: obj.Address,
			Name:    obj.AddressResourceName,
			Version: obj.AddressVersion,
		}
	}, krtopts.ToOptions("XDSAddresses")...)
}

func (s *Syncer) buildXDSCollection(
	agwResources krt.Collection[agwir.AgwResourcesForGateway],
	agwBackends krt.Collection[translator.AgwResourceWithCustomName],
	xdsAddresses krt.Collection[translator.AgwResourceWithCustomName],
	krtopts krtutil.KrtOptions,
) {
	// Create an index on agwResources by Gateway to avoid fetching all resources
	agwResourcesByGateway := krt.NewIndex(agwResources, "gateway", func(resource agwir.AgwResourcesForGateway) []types.NamespacedName {
		return []types.NamespacedName{resource.Gateway}
	})
	s.xDS = krt.NewCollection(agwResources, func(kctx krt.HandlerContext, obj agwir.AgwResourcesForGateway) *translator.AgentGwXdsResources {
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
		resourceList := krt.Fetch(kctx, agwResources, krt.FilterIndex(agwResourcesByGateway, gwNamespacedName))
		for _, resource := range resourceList {
			// 1. merge GW Reports for all Proxies' status reports
			maps.Copy(gwReports.Gateways, resource.Report.Gateways)

			// 2. merge LS Reports for all Proxies' status reports
			maps.Copy(gwReports.ListenerSets, resource.Report.ListenerSets)

			// 3. merge route parentRefs into RouteReports for all route types
			mergeRouteReports(gwReports.HTTPRoutes, resource.Report.HTTPRoutes)
			mergeRouteReports(gwReports.TCPRoutes, resource.Report.TCPRoutes)
			mergeRouteReports(gwReports.TLSRoutes, resource.Report.TLSRoutes)
			mergeRouteReports(gwReports.GRPCRoutes, resource.Report.GRPCRoutes)

			for key, rr := range resource.Report.Policies {
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
				cacheResources = append(cacheResources, &translator.AgwResourceWithCustomName{
					Message: res,
					Name:    agwir.GetAgwResourceName(res),
					Version: utils.HashProto(res),
				})
			}
			for listenerName, count := range resource.AttachedRoutes {
				attachedRoutes[listenerName] += count
			}
		}

		// Fetch all backends and add them to the resources for every gateway
		cachedBackends := krt.Fetch(kctx, agwBackends)
		for _, backend := range cachedBackends {
			cacheResources = append(cacheResources, &backend)
		}

		// Create the resource wrappers
		var resourceVersion uint64
		for _, res := range cacheResources {
			resourceVersion ^= res.(*translator.AgwResourceWithCustomName).Version
		}
		// Calculate address version
		var addrVersion uint64
		for _, res := range cacheAddresses {
			addrVersion ^= res.Version
		}

		var gwObservedGen int64
		if gen, ok := (&gwReports).GatewayObservedGenerationFor(gwNamespacedName); ok {
			gwObservedGen = gen
		}

		result := &translator.AgentGwXdsResources{
			NamespacedName:    gwNamespacedName,
			Reports:           gwReports,
			AttachedRoutes:    attachedRoutes,
			GatewayGeneration: gwObservedGen,
			ResourceConfig:    envoycache.NewResources(fmt.Sprintf("%d", resourceVersion), cacheResources),
			AddressConfig:     envoycache.NewResources(fmt.Sprintf("%d", addrVersion), envoytypesAddresses),
		}
		logger.Debug("created XDS resources for gateway with ID", "gwname", fmt.Sprintf("%s,%s", gwNamespacedName.Name, gwNamespacedName.Namespace), "resourceid", result.ResourceName())
		return result
	}, krtopts.ToOptions("agent-xds")...)
}

func (s *Syncer) buildStatusReporting(policyStatuses map[schema.GroupKind]krt.StatusCollection[controllers.Object, gwv1alpha2.PolicyStatus]) {
	// TODO(npolshak): Move away from report map and separately fetch resource reports
	// Create separate singleton collections for each resource type instead of merging everything
	// This avoids the overhead of creating and processing a single large merged report
	gatewayReports := krt.NewSingleton(func(kctx krt.HandlerContext) *translator.GatewayReports {
		proxies := krt.Fetch(kctx, s.xDS)
		merged := make(map[types.NamespacedName]*reports.GatewayReport)

		attached := make(map[types.NamespacedName]map[string]uint)
		generations := make(map[types.NamespacedName]int64)
		for _, p := range proxies {
			// merge GW status reports
			if gwRep, ok := p.Reports.Gateways[p.NamespacedName]; ok {
				merged[p.NamespacedName] = gwRep
			}
			// record observed generation for this gateway to update report
			generations[p.NamespacedName] = p.GatewayGeneration
			// take max per listener across proxies
			if attached[p.NamespacedName] == nil {
				attached[p.NamespacedName] = make(map[string]uint)
			}
			for lis, c := range p.AttachedRoutes {
				if c > attached[p.NamespacedName][lis] {
					attached[p.NamespacedName][lis] = c
				}
			}
		}
		return &translator.GatewayReports{
			Reports:        merged,
			AttachedRoutes: attached,
			Generations:    generations,
		}
	})

	listenerSetReports := krt.NewSingleton(func(kctx krt.HandlerContext) *translator.ListenerSetReports {
		proxies := krt.Fetch(kctx, s.xDS)
		merged := make(map[types.NamespacedName]*reports.ListenerSetReport)

		for _, p := range proxies {
			// Merge LS Reports for all Proxies' status reports
			maps.Copy(merged, p.Reports.ListenerSets)
		}

		return &translator.ListenerSetReports{
			Reports: merged,
		}
	})

	routeReports := krt.NewSingleton(func(kctx krt.HandlerContext) *translator.RouteReports {
		proxies := krt.Fetch(kctx, s.xDS)
		merged := translator.RouteReports{
			HTTPRoutes: make(map[types.NamespacedName]*reports.RouteReport),
			GRPCRoutes: make(map[types.NamespacedName]*reports.RouteReport),
			TCPRoutes:  make(map[types.NamespacedName]*reports.RouteReport),
			TLSRoutes:  make(map[types.NamespacedName]*reports.RouteReport),
		}

		for _, p := range proxies {
			// Merge route parentRefs into RouteReports for all route types
			mergeRouteReports(merged.HTTPRoutes, p.Reports.HTTPRoutes)
			mergeRouteReports(merged.GRPCRoutes, p.Reports.GRPCRoutes)
			mergeRouteReports(merged.TCPRoutes, p.Reports.TCPRoutes)
			mergeRouteReports(merged.TLSRoutes, p.Reports.TLSRoutes)
		}

		return &merged
	})

	// Store references to the separate collections
	s.gatewayReports = gatewayReports
	s.listenerSetReports = listenerSetReports
	s.routeReports = routeReports

	// Register policy status collection with the policy status queue
	registerPolicyStatus(s.policyStatusQueue, policyStatuses)
}

// registerPolicyStatus takes a policy status collection and registers it to be managed by Istio's StatusCollections.
func registerPolicyStatus(s *status.StatusCollections, statusCols map[schema.GroupKind]krt.StatusCollection[controllers.Object, gwv1alpha2.PolicyStatus]) {
	for gvk, statusCol := range statusCols {
		// Capture the GVK for the closure
		currentGVK := gvk
		currentStatusCol := statusCol

		// Create a writer function that matches Istio's StatusCollections interface
		writer := func(queue status.Queue) krt.HandlerRegistration {
			// Register the status collection to write to the queue
			h := currentStatusCol.Register(func(o krt.Event[krt.ObjectWithStatus[controllers.Object, gwv1alpha2.PolicyStatus]]) {
				l := o.Latest()

				// Cast controllers.Object to TrafficPolicy for validation (following the pattern requested)
				switch currentGVK.Kind {
				case "TrafficPolicy":
					if _, ok := l.Obj.(*v1alpha1.TrafficPolicy); !ok {
						logger.Error("failed to cast to TrafficPolicy", "resource", l.ResourceName(), "kind", currentGVK.Kind)
						return
					}
				default:
					// For other policy types that might be added in the future
					logger.Debug("handling policy type", "kind", currentGVK.Kind, "resource", l.ResourceName())
				}

				if o.Event == controllers.EventDelete {
					// if the object is being deleted, we should not reset status
					return
				}
				// Create a status.Resource from our object and pass the object as context
				resource := status.Resource{
					Name:      l.Obj.GetName(),
					Namespace: l.Obj.GetNamespace(),
				}
				queue.EnqueueStatusUpdateResource(l, resource)
				logger.Debug("enqueued policy status update", "resource", l.ResourceName(), "version", l.Obj.GetResourceVersion(), "status", l.Status, "kind", currentGVK.Kind)
			})
			return h
		}
		s.Register(writer)
	}
}

func (s *Syncer) setupSyncDependencies(gateways krt.Collection[translator.GatewayListener], agwResources krt.Collection[agwir.AgwResourcesForGateway], agwBackends krt.Collection[translator.AgwResourceWithCustomName], addresses krt.Collection[translator.AgwResourceWithCustomName]) {
	s.waitForSync = []cache.InformerSynced{
		s.agwCollections.HasSynced,
		s.agwPlugins.HasSynced,
		gateways.HasSynced,
		// resources
		agwResources.HasSynced,
		agwBackends.HasSynced,
		s.xDS.HasSynced,
		// addresses
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

	// wait for ctrl-rtime caches to sync before accepting events
	if !s.mgr.GetCache().WaitForCacheSync(ctx) {
		return fmt.Errorf("agent gateway sync loop waiting for all caches to sync failed")
	}
	logger.Info("caches warm!")

	// Register to separate singleton collections instead of a single merged report
	s.gatewayReports.Register(func(o krt.Event[translator.GatewayReports]) {
		if o.Event == controllers.EventDelete {
			// TODO: handle garbage collection
			return
		}
		s.gatewayReportQueue.Enqueue(o.Latest())
	})

	s.listenerSetReports.Register(func(o krt.Event[translator.ListenerSetReports]) {
		if o.Event == controllers.EventDelete {
			// TODO: handle garbage collection
			return
		}
		s.listenerSetReportQueue.Enqueue(o.Latest())
	})

	s.routeReports.Register(func(o krt.Event[translator.RouteReports]) {
		if o.Event == controllers.EventDelete {
			// TODO: handle garbage collection
			return
		}
		s.routeReportQueue.Enqueue(o.Latest())
	})

	s.xDS.RegisterBatch(func(events []krt.Event[translator.AgentGwXdsResources]) {
		for _, e := range events {
			snap := e.Latest()
			if e.Event == controllers.EventDelete {
				// TODO: we should probably clear, but this has been causing some undiagnosed issues.
				// s.xdsCache.ClearSnapshot(snap.ResourceName())
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

func (s *Syncer) HasSynced() bool {
	return s.ready.Load()
}

// NeedLeaderElection returns false to ensure that the Syncer runs on all pods (leader and followers)
func (r *Syncer) NeedLeaderElection() bool {
	return false
}

// ReportQueue returns the queue that contains the latest GatewayReports.
// It will be constantly updated to contain the merged status report for Kube Gateway status.
func (s *Syncer) GatewayReportQueue() utils.AsyncQueue[translator.GatewayReports] {
	return s.gatewayReportQueue
}

// ListenerSetReportQueue returns the queue that contains the latest ListenerSetReports.
// It will be constantly updated to contain the merged status report for Kube Gateway status.
func (s *Syncer) ListenerSetReportQueue() utils.AsyncQueue[translator.ListenerSetReports] {
	return s.listenerSetReportQueue
}

// RouteReportQueue returns the queue that contains the latest RouteReports.
// It will be constantly updated to contain the merged status report for Kube Gateway status.
func (s *Syncer) RouteReportQueue() utils.AsyncQueue[translator.RouteReports] {
	return s.routeReportQueue
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
