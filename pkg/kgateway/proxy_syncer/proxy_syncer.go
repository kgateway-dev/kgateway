package proxy_syncer

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	kmetrics "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	plug "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

var _ manager.LeaderElectionRunnable = &ProxySyncer{}

// ProxySyncer orchestrates the translation of K8s Gateway CRs to xDS
// and setting the output xDS snapshot in the envoy snapshot cache,
// resulting in each connected proxy getting the correct configuration.
// It runs on all pods (leader or follower) as the xDS snapshot must be consistent across pods.
// It also derives per-object desired-status collections from the translation reports;
// the StatusSyncer attaches them to a write queue on the leader.
type ProxySyncer struct {
	controllerName string

	mgr        manager.Manager
	commonCols *collections.CommonCollections
	translator *translator.CombinedTranslator
	plugins    plug.Plugin

	apiClient       apiclient.Client
	proxyTranslator ProxyTranslator

	uniqueClients krt.Collection[ir.UniquelyConnectedClient]

	statusReport            krt.Singleton[statussync.ReportsWrapper]
	backendPolicyReport     krt.Singleton[statussync.ReportsWrapper]
	backendStatusReport     krt.Singleton[statussync.ReportsWrapper]
	policyReport            krt.Singleton[statussync.ReportsWrapper]
	mostXdsSnapshots        krt.Collection[GatewayXdsResources]
	perclientSnapCollection krt.Collection[XdsSnapWrapper]

	statusCollections *statussync.StatusCollections
	statusWriters     map[schema.GroupVersionKind]statussync.ResourceStatusSyncer

	waitForSync []cache.InformerSynced
	ready       atomic.Bool
}

type GatewayXdsResources struct {
	types.NamespacedName

	reports reports.ReportMap
	// Clusters are items in the CDS response payload.
	// +krtEqualsTodo include CDC resources in equality for diff detection
	Clusters     []envoycachetypes.ResourceWithTTL
	ClustersHash uint64

	// Routes are items in the RDS response payload.
	Routes envoycache.Resources

	// Listeners are items in the LDS response payload.
	Listeners envoycache.Resources

	// Secrets are items in the SDS response payload.
	Secrets envoycache.Resources
}

func (r GatewayXdsResources) ResourceName() string {
	return xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, r.Namespace, r.Name)
}

func (r GatewayXdsResources) Equals(in GatewayXdsResources) bool {
	return r.NamespacedName == in.NamespacedName &&
		reports.EqualReportMaps(r.reports, in.reports) &&
		r.ClustersHash == in.ClustersHash &&
		r.Routes.Version == in.Routes.Version &&
		r.Listeners.Version == in.Listeners.Version &&
		r.Secrets.Version == in.Secrets.Version
}

func sliceToResourcesHash[T proto.Message](slice []T) ([]envoycachetypes.ResourceWithTTL, uint64) {
	var slicePb []envoycachetypes.ResourceWithTTL
	var resourcesHash uint64
	for _, r := range slice {
		var m proto.Message = r
		hash := utils.HashProto(r)
		slicePb = append(slicePb, envoycachetypes.ResourceWithTTL{Resource: m})
		resourcesHash ^= hash
	}

	return slicePb, resourcesHash
}

func sliceToResources[T proto.Message](slice []T) envoycache.Resources {
	r, h := sliceToResourcesHash(slice)
	return envoycache.NewResourcesWithTTL(fmt.Sprintf("%d", h), r)
}

func toResources(gw ir.Gateway, xdsSnap irtranslator.TranslationResult, r reports.ReportMap) *GatewayXdsResources {
	c, ch := sliceToResourcesHash(xdsSnap.ExtraClusters)
	return &GatewayXdsResources{
		NamespacedName: types.NamespacedName{
			Namespace: gw.Obj.GetNamespace(),
			Name:      gw.Obj.GetName(),
		},
		reports:      r,
		ClustersHash: ch,
		Clusters:     c,
		Routes:       sliceToResources(xdsSnap.Routes),
		Listeners:    sliceToResources(xdsSnap.Listeners),
		Secrets:      sliceToResources(xdsSnap.Secrets),
	}
}

// NewProxySyncer returns a ProxySyncer runnable
// The provided GatewayInputChannels are used to trigger syncs.
func NewProxySyncer(
	ctx context.Context,
	controllerName string,
	mgr manager.Manager,
	client apiclient.Client,
	uniqueClients krt.Collection[ir.UniquelyConnectedClient],
	mergedPlugins plug.Plugin,
	commonCols *collections.CommonCollections,
	xdsCache envoycache.SnapshotCache,
	validator validator.Validator,
) *ProxySyncer {
	return &ProxySyncer{
		controllerName:  controllerName,
		commonCols:      commonCols,
		mgr:             mgr,
		apiClient:       client,
		proxyTranslator: NewProxyTranslator(xdsCache),
		uniqueClients:   uniqueClients,
		translator:      translator.NewCombinedTranslator(ctx, mergedPlugins, commonCols, validator),
		plugins:         mergedPlugins,
	}
}

type ProxyTranslator struct {
	xdsCache envoycache.SnapshotCache
}

func NewProxyTranslator(xdsCache envoycache.SnapshotCache) ProxyTranslator {
	return ProxyTranslator{
		xdsCache: xdsCache,
	}
}

var logger = logging.New("proxy_syncer")

func (s *ProxySyncer) Init(ctx context.Context, krtopts krtutil.KrtOptions) {
	queries := query.NewData(s.commonCols)

	gatewayBackendVariants := newGatewayBackendVariants(
		ctx,
		krtopts,
		queries,
		s.commonCols.GatewayIndex.Gateways,
	)
	gatewayBackendVariantBackends := krt.NewCollection(gatewayBackendVariants, func(kctx krt.HandlerContext, backendForGateway gatewayScopedBackend) *ir.BackendObjectIR {
		if backendForGateway.backend == nil {
			return nil
		}
		backend := *backendForGateway.backend
		return &backend
	}, krtopts.ToOptions("GatewayBackendClientCertificateVariantBackends")...)
	gatewayBackendVariantBackendsWithPolicy, _ := s.commonCols.BackendIndex.AttachPoliciesToCollection(
		gatewayBackendVariantBackends,
		"GatewayBackendClientCertificateVariantBackendsWithPolicy",
	)
	gatewayBackendVariantEndpoints := newGatewayBackendVariantEndpoints(krtopts, gatewayBackendVariants, s.commonCols.Endpoints)

	// all backends with policies attached in a single collection
	finalBackends := krt.JoinCollection(
		append(s.commonCols.BackendIndex.BackendsWithPolicy(), gatewayBackendVariantBackendsWithPolicy),
		// WithJoinUnchecked enables a more optimized lookup on the hotpath by assuming we do not have any overlapping ResourceName
		// in the backend collection.
		append(krtopts.ToOptions("FinalBackends"), krt.WithJoinUnchecked())...)
	finalBackendsWithPolicyStatus := krt.JoinCollection(s.commonCols.BackendIndex.BackendsWithPolicyRequiringStatus(),
		// WithJoinUnchecked enables a more optimized lookup on the hotpath by assuming we do not have any overlapping ResourceName
		// in the backend collection.
		append(krtopts.ToOptions("FinalBackendsWithPolicyStatus"), krt.WithJoinUnchecked())...)
	allEndpoints := krt.JoinCollection(
		[]krt.Collection[ir.EndpointsForBackend]{s.commonCols.Endpoints, gatewayBackendVariantEndpoints},
		krtopts.ToOptions("AllEndpoints")...,
	)

	s.translator.Init(ctx)

	s.mostXdsSnapshots = krt.NewCollection(s.commonCols.GatewayIndex.Gateways, func(kctx krt.HandlerContext, gw ir.Gateway) *GatewayXdsResources {
		// Note: s.commonCols.GatewayIndex.Gateways is already filtered to only include Gateways
		// with controllerName matching s.controllerName (envoy controller). The filtering happens
		// in GatewaysForEnvoyTransformationFunc in pkg/krtcollections/policy.go
		logger.Debug("building proxy for kube gw", "name", client.ObjectKeyFromObject(gw.Obj), "version", gw.Obj.GetResourceVersion())

		xdsSnap, rm := s.translator.TranslateGateway(kctx, ctx, gw)
		if xdsSnap == nil {
			return nil
		}

		return toResources(gw, *xdsSnap, rm)
	}, krtopts.ToOptions("MostXdsSnapshots")...)

	epPerClient := NewPerClientEnvoyEndpoints(
		krtopts,
		s.uniqueClients,
		newFinalBackendEndpoints(krtopts, finalBackends, allEndpoints),
		s.translator.TranslateEndpoints,
	)
	localClusterEpPerClient := NewPerClientLocalClusterEndpoints(
		krtopts,
		s.uniqueClients,
		s.commonCols.LocalityPods,
	)
	clustersPerClient := NewPerClientEnvoyClusters(
		ctx,
		krtopts,
		s.translator.GetBackendTranslator(),
		finalBackends,
		s.uniqueClients,
	)

	s.perclientSnapCollection = snapshotPerClient(
		krtopts,
		s.uniqueClients,
		s.mostXdsSnapshots,
		epPerClient,
		clustersPerClient,
		localClusterEpPerClient,
	)

	excludedPolicyKinds := make(map[schema.GroupKind]struct{})
	for gk, plugin := range s.plugins.ContributesPolicies {
		if plugin.PolicyStatusFromGatewayReports {
			excludedPolicyKinds[gk] = struct{}{}
		}
	}

	s.backendPolicyReport = krt.NewSingleton(func(kctx krt.HandlerContext) *statussync.ReportsWrapper {
		backends := krt.Fetch(kctx, finalBackendsWithPolicyStatus)
		merged := GenerateBackendPolicyReport(backends, excludedPolicyKinds)

		for _, plugin := range s.plugins.ContributesPolicies {
			if plugin.ProcessPolicyStaleStatusMarkers != nil && plugin.ProcessBackend != nil && !plugin.PolicyStatusFromGatewayReports {
				plugin.ProcessPolicyStaleStatusMarkers(kctx, &merged)
			}
		}

		w := statussync.NewReportsWrapper(merged)
		return &w
	}, krtopts.ToOptions("BackendsPolicyReport")...)

	// backendStatusReport is the sole writer of the Backend Accepted condition: it merges
	// each Backend's IR errors with its per-client translation errors. It also merges any
	// plugin-contributed conditions (e.g. the EC2 EndpointsDiscovered condition) so all
	// Backend conditions are written by a single owner.
	kgwBackendPlugin := s.plugins.ContributesBackends[wellknown.BackendGVK.GroupKind()]
	kgwBackendCol := kgwBackendPlugin.Backends
	kgwBackendExtraConditions := kgwBackendPlugin.ExtraConditions
	s.backendStatusReport = krt.NewSingleton(func(kctx krt.HandlerContext) *statussync.ReportsWrapper {
		var kgwBackends []ir.BackendObjectIR
		if kgwBackendCol != nil {
			kgwBackends = krt.Fetch(kctx, kgwBackendCol)
		}
		clusters := krt.Fetch(kctx, clustersPerClient.clusters)
		var extraConditions []ir.BackendObjectStatus
		if kgwBackendExtraConditions != nil {
			extraConditions = krt.Fetch(kctx, kgwBackendExtraConditions)
		}
		merged := GenerateBackendStatusReport(kgwBackends, clusters, extraConditions)
		w := statussync.NewReportsWrapper(merged)
		return &w
	}, krtopts.ToOptions("BackendStatusReport")...)

	// as proxies are created, they also contain a reportMap containing status for the Gateway and associated xRoutes (really parentRefs)
	// here we will merge reports that are per-Proxy to a singleton Report used to persist to k8s on a timer
	s.statusReport = krt.NewSingleton(func(kctx krt.HandlerContext) *statussync.ReportsWrapper {
		proxies := krt.Fetch(kctx, s.mostXdsSnapshots)

		merged := mergeProxyReports(proxies)

		// Process status markers
		s.commonCols.Routes.ProcessRouteStatusMarkers(kctx, merged)

		for _, plugin := range s.plugins.ContributesPolicies {
			if plugin.ProcessPolicyStaleStatusMarkers != nil && (plugin.ProcessBackend == nil || plugin.PolicyStatusFromGatewayReports) {
				plugin.ProcessPolicyStaleStatusMarkers(kctx, &merged)
			}
		}

		w := statussync.NewReportsWrapper(merged)
		return &w
	})

	// policyReport merges the gateway-translation and backend-attachment report paths so a
	// policy's status is built from the union of its ancestors, no matter which path
	// produced them. (The previous syncer wrote the two paths independently, which could
	// race when a policy was attached via both.)
	s.policyReport = krt.NewSingleton(func(kctx krt.HandlerContext) *statussync.ReportsWrapper {
		merged := []reports.ReportMap{}
		if gwReports := krt.FetchOne(kctx, s.statusReport.AsCollection()); gwReports != nil {
			merged = append(merged, gwReports.Reports())
		}
		if backendReports := krt.FetchOne(kctx, s.backendPolicyReport.AsCollection()); backendReports != nil {
			merged = append(merged, backendReports.Reports())
		}
		w := statussync.NewReportsWrapper(reports.MergeReportMaps(merged...))
		return &w
	}, krtopts.ToOptions("PolicyStatusReport")...)

	s.waitForSync = []cache.InformerSynced{
		s.commonCols.HasSynced,
		finalBackends.HasSynced,
		s.perclientSnapCollection.HasSynced,
		s.mostXdsSnapshots.HasSynced,
		s.plugins.HasSynced,
		s.translator.HasSynced,
	}

	s.initStatusInfra(ctx, krtopts)
}

func mergeProxyReports(
	proxies []GatewayXdsResources,
) reports.ReportMap {
	inputs := make([]reports.ReportMap, 0, len(proxies))
	for _, proxy := range proxies {
		inputs = append(inputs, proxy.reports)
	}
	return reports.MergeReportMaps(inputs...)
}

func (s *ProxySyncer) Start(ctx context.Context) error {
	logger.Info("starting Proxy Syncer", "controller", s.controllerName)

	// wait for krt collections to sync
	logger.Info("waiting for cache to sync")
	s.apiClient.WaitForCacheSync(
		"kube gw proxy syncer",
		ctx.Done(),
		s.waitForSync...,
	)

	// wait for ctrl-rtime caches to sync before accepting events
	if !s.mgr.GetCache().WaitForCacheSync(ctx) {
		return errors.New("kube gateway proxy syncer sync loop waiting for all caches to sync failed")
	}
	logger.Info("caches warm!")

	// caches are warm, now we can do registrations
	s.perclientSnapCollection.RegisterBatch(func(o []krt.Event[XdsSnapWrapper]) {
		for _, e := range o {
			cd := getDetailsFromXDSClientResourceName(e.Latest().ResourceName())

			if e.Event != controllers.EventDelete {
				snapWrap := e.Latest()
				s.proxyTranslator.syncXds(ctx, snapWrap)
			} else {
				// Intentional no-op. When snapshotPerClient returns nil (its
				// per-client inputs weren't derived yet, so it deferred
				// publishing), KRT surfaces a Delete for this UCC. Clearing
				// the xDS cache here would withdraw Envoy's last coherent
				// Snapshot for the duration of the defer, causing 500/NC on
				// valid routes. Leaving the cache alone means Envoy keeps
				// serving its previously-published config until a new
				// snapshot overwrites it — "retain last good".
				//
				// Known leak: this branch also fires when a UCC truly goes
				// away (Envoy pod replaced on rollout, scaled down, etc.),
				// and we cannot distinguish that from the "defer" case here.
				// The SnapshotCache entry for that UCC is therefore never
				// cleared and accumulates over the controller's lifetime.
				// Pre-existing behavior (the prior ClearSnapshot call was
				// already commented out); reclaiming these entries requires
				// a separate signal — e.g. cross-referencing uccCol
				// membership — and is left to a follow-up.
			}

			kmetrics.EndResourceXDSSync(kmetrics.ResourceSyncDetails{
				Namespace:    cd.Namespace,
				Gateway:      cd.Gateway,
				ResourceName: cd.Gateway,
			})
		}
	}, true)

	s.ready.Store(true)
	<-ctx.Done()
	return nil
}

func (s *ProxySyncer) HasSynced() bool {
	return s.ready.Load()
}

// NeedLeaderElection returns false to ensure that the proxySyncer runs on all pods (leader and followers)
func (r *ProxySyncer) NeedLeaderElection() bool {
	return false
}

// StatusCollections returns the registered desired-status collections. The status syncer
// enables them (attaching handlers that feed the write queue) on the leader.
func (s *ProxySyncer) StatusCollections() *statussync.StatusCollections {
	return s.statusCollections
}

// StatusWriters returns the per-GVK status writers used to persist desired statuses.
func (s *ProxySyncer) StatusWriters() map[schema.GroupVersionKind]statussync.ResourceStatusSyncer {
	return s.statusWriters
}

// GatewayReports returns the merged gateway report singleton as a collection; used by the
// status syncer's custom status sync hook.
func (s *ProxySyncer) GatewayReports() krt.Collection[statussync.ReportsWrapper] {
	return s.statusReport.AsCollection()
}

// WaitForSync returns a list of functions that can be used to determine if all its informers have synced.
// This is useful for determining if caches have synced.
// It must be called only after `Init()`.
func (s *ProxySyncer) CacheSyncs() []cache.InformerSynced {
	return s.waitForSync
}

type resourcesStringer envoycache.Resources

func (r resourcesStringer) String() string {
	return fmt.Sprintf("len: %d, version %s", len(r.Items), r.Version)
}
