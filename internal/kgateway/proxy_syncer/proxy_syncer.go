package proxy_syncer

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync/atomic"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	kmetrics "github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections/metrics"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	plug "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

var _ manager.LeaderElectionRunnable = &ProxySyncer{}

// ProxySyncer orchestrates the translation of K8s Gateway CRs to xDS
// and setting the output xDS snapshot in the envoy snapshot cache,
// resulting in each connected proxy getting the correct configuration.
// It runs on all pods (leader or follower) as the xDS snapshot must be consistent across pods.
// It queues the status reports resulting from translation on the `reportQueue` && `backendPolicyReportQueue`
// to be handled by the statusSyncer.
type ProxySyncer struct {
	controllerName        string
	agentGatewayClassName string

	mgr        manager.Manager
	commonCols *common.CommonCollections
	translator *translator.CombinedTranslator
	plugins    plug.Plugin

	istioClient     kube.Client
	proxyTranslator ProxyTranslator

	uniqueClients krt.Collection[ir.UniqlyConnectedClient]

	statusReport            krt.Singleton[report]
	backendPolicyReport     krt.Singleton[report]
	mostXdsSnapshots        krt.Collection[GatewayXdsResources]
	perclientSnapCollection krt.Collection[XdsSnapWrapper]

	waitForSync []cache.InformerSynced
	ready       atomic.Bool

	reportQueue              utils.AsyncQueue[reports.ReportMap]
	backendPolicyReportQueue utils.AsyncQueue[reports.ReportMap]
}

type GatewayXdsResources struct {
	types.NamespacedName

	reports reports.ReportMap
	// Clusters are items in the CDS response payload.
	Clusters     []envoycachetypes.ResourceWithTTL
	ClustersHash uint64

	// Routes are items in the RDS response payload.
	Routes envoycache.Resources

	// Listeners are items in the LDS response payload.
	Listeners envoycache.Resources
}

func (r GatewayXdsResources) ResourceName() string {
	return xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, r.Namespace, r.Name)
}

func (r GatewayXdsResources) Equals(in GatewayXdsResources) bool {
	return r.NamespacedName == in.NamespacedName &&
		report{r.reports}.Equals(report{in.reports}) &&
		r.ClustersHash == in.ClustersHash &&
		r.Routes.Version == in.Routes.Version &&
		r.Listeners.Version == in.Listeners.Version
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
	}
}

// NewProxySyncer returns a ProxySyncer runnable
// The provided GatewayInputChannels are used to trigger syncs.
func NewProxySyncer(
	ctx context.Context,
	controllerName string,
	mgr manager.Manager,
	client kube.Client,
	uniqueClients krt.Collection[ir.UniqlyConnectedClient],
	mergedPlugins plug.Plugin,
	commonCols *common.CommonCollections,
	xdsCache envoycache.SnapshotCache,
	agentGatewayClassName string,
	validator validator.Validator,
) *ProxySyncer {
	return &ProxySyncer{
		controllerName:           controllerName,
		agentGatewayClassName:    agentGatewayClassName,
		commonCols:               commonCols,
		mgr:                      mgr,
		istioClient:              client,
		proxyTranslator:          NewProxyTranslator(xdsCache),
		uniqueClients:            uniqueClients,
		translator:               translator.NewCombinedTranslator(ctx, mergedPlugins, commonCols, validator),
		plugins:                  mergedPlugins,
		reportQueue:              utils.NewAsyncQueue[reports.ReportMap](),
		backendPolicyReportQueue: utils.NewAsyncQueue[reports.ReportMap](),
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

type report struct {
	// lower case so krt doesn't error in debug handler
	reportMap reports.ReportMap
}

func (r report) ResourceName() string {
	return "report"
}

// do we really need this for a singleton?
func (r report) Equals(in report) bool {
	if !maps.Equal(r.reportMap.Gateways, in.reportMap.Gateways) {
		return false
	}
	if !maps.Equal(r.reportMap.ListenerSets, in.reportMap.ListenerSets) {
		return false
	}
	if !maps.Equal(r.reportMap.HTTPRoutes, in.reportMap.HTTPRoutes) {
		return false
	}
	if !maps.Equal(r.reportMap.TCPRoutes, in.reportMap.TCPRoutes) {
		return false
	}
	if !maps.Equal(r.reportMap.TLSRoutes, in.reportMap.TLSRoutes) {
		return false
	}
	if !maps.Equal(r.reportMap.Policies, in.reportMap.Policies) {
		return false
	}
	return true
}

var logger = logging.New("proxy_syncer")

func (s *ProxySyncer) Init(ctx context.Context, krtopts krtinternal.KrtOptions) {
	// all backends with policies attached in a single collection
	finalBackends := krt.JoinCollection(s.commonCols.BackendIndex.BackendsWithPolicy(),
		// WithJoinUnchecked enables a more optimized lookup on the hotpath by assuming we do not have any overlapping ResourceName
		// in the backend collection.
		append(krtopts.ToOptions("FinalBackends"), krt.WithJoinUnchecked())...)

	s.translator.Init(ctx)

	s.mostXdsSnapshots = krt.NewCollection(s.commonCols.GatewayIndex.Gateways, func(kctx krt.HandlerContext, gw ir.Gateway) *GatewayXdsResources {
		// skip agentgateway proxies as they are not envoy-based gateways
		if string(gw.Obj.Spec.GatewayClassName) == s.agentGatewayClassName {
			logger.Debug("skipping envoy proxy sync for agentgateway %s.%s", gw.Obj.Name, gw.Obj.Namespace)
			return nil
		}

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
		s.commonCols.Endpoints,
		s.translator.TranslateEndpoints,
	)
	clustersPerClient := NewPerClientEnvoyClusters(
		ctx,
		krtopts,
		s.translator.GetUpstreamTranslator(),
		finalBackends,
		s.uniqueClients,
	)

	s.perclientSnapCollection = snapshotPerClient(
		krtopts,
		s.uniqueClients,
		s.mostXdsSnapshots,
		epPerClient,
		clustersPerClient,
	)

	s.backendPolicyReport = krt.NewSingleton(func(kctx krt.HandlerContext) *report {
		backends := krt.Fetch(kctx, finalBackends)
		merged := GenerateBackendPolicyReport(backends)
		return &report{merged}
	}, krtopts.ToOptions("BackendsPolicyReport")...)

	// as proxies are created, they also contain a reportMap containing status for the Gateway and associated xRoutes (really parentRefs)
	// here we will merge reports that are per-Proxy to a singleton Report used to persist to k8s on a timer
	s.statusReport = krt.NewSingleton(func(kctx krt.HandlerContext) *report {
		proxies := krt.Fetch(kctx, s.mostXdsSnapshots)
		merged := mergeProxyReports(proxies)
		return &report{merged}
	})

	s.waitForSync = []cache.InformerSynced{
		s.commonCols.HasSynced,
		finalBackends.HasSynced,
		s.perclientSnapCollection.HasSynced,
		s.mostXdsSnapshots.HasSynced,
		s.plugins.HasSynced,
		s.translator.HasSynced,
	}
}

func mergeProxyReports(
	proxies []GatewayXdsResources,
) reports.ReportMap {
	merged := reports.NewReportMap()
	for _, p := range proxies {
		// 1. merge GW Reports for all Proxies' status reports
		maps.Copy(merged.Gateways, p.reports.Gateways)

		// 2. merge LS Reports for all Proxies' status reports
		maps.Copy(merged.ListenerSets, p.reports.ListenerSets)

		// 3. merge httproute parentRefs into RouteReports
		for rnn, rr := range p.reports.HTTPRoutes {
			// if we haven't encountered this route, just copy it over completely
			old := merged.HTTPRoutes[rnn]
			if old == nil {
				merged.HTTPRoutes[rnn] = rr
				continue
			}
			// else, this route has already been seen for a proxy, merge this proxy's parents
			// into the merged report
			maps.Copy(merged.HTTPRoutes[rnn].Parents, rr.Parents)
		}

		// 4. merge tcproute parentRefs into RouteReports
		for rnn, rr := range p.reports.TCPRoutes {
			// if we haven't encountered this route, just copy it over completely
			old := merged.TCPRoutes[rnn]
			if old == nil {
				merged.TCPRoutes[rnn] = rr
				continue
			}
			// else, this route has already been seen for a proxy, merge this proxy's parents
			// into the merged report
			maps.Copy(merged.TCPRoutes[rnn].Parents, rr.Parents)
		}

		for rnn, rr := range p.reports.TLSRoutes {
			// if we haven't encountered this route, just copy it over completely
			old := merged.TLSRoutes[rnn]
			if old == nil {
				merged.TLSRoutes[rnn] = rr
				continue
			}
			// else, this route has already been seen for a proxy, merge this proxy's parents
			// into the merged report
			maps.Copy(merged.TLSRoutes[rnn].Parents, rr.Parents)
		}

		for rnn, rr := range p.reports.GRPCRoutes {
			// if we haven't encountered this route, just copy it over completely
			old := merged.GRPCRoutes[rnn]
			if old == nil {
				merged.GRPCRoutes[rnn] = rr
				continue
			}
			// else, this route has already been seen for a proxy, merge this proxy's parents
			// into the merged report
			maps.Copy(merged.GRPCRoutes[rnn].Parents, rr.Parents)
		}

		for key, report := range p.reports.Policies {
			// if we haven't encountered this policy, just copy it over completely
			old := merged.Policies[key]
			if old == nil {
				merged.Policies[key] = report
				continue
			}
			// else, let's merge our parentRefs into the existing map
			// obsGen will stay as-is...
			maps.Copy(merged.Policies[key].Ancestors, report.Ancestors)
		}
	}

	return merged
}

func (s *ProxySyncer) Start(ctx context.Context) error {
	logger.Info("starting Proxy Syncer", "controller", s.controllerName)

	// wait for krt collections to sync
	logger.Info("waiting for cache to sync")
	s.istioClient.WaitForCacheSync(
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

	// latestReport will be constantly updated to contain the merged status report for Kube Gateway status
	// when timer ticks, we will use the state of the mergedReports at that point in time to sync the status to k8s
	s.statusReport.Register(func(o krt.Event[report]) {
		if o.Event == controllers.EventDelete {
			// TODO: handle garbage collection (see: https://github.com/solo-io/solo-projects/issues/7086)
			return
		}
		s.reportQueue.Enqueue(o.Latest().reportMap)
	})

	s.backendPolicyReport.Register(func(o krt.Event[report]) {
		if o.Event == controllers.EventDelete {
			return
		}
		s.backendPolicyReportQueue.Enqueue(o.Latest().reportMap)
	})

	s.perclientSnapCollection.RegisterBatch(func(o []krt.Event[XdsSnapWrapper]) {
		for _, e := range o {
			cd := getDetailsFromXDSClientResourceName(e.Latest().ResourceName())

			if e.Event != controllers.EventDelete {
				snapWrap := e.Latest()
				s.proxyTranslator.syncXds(ctx, snapWrap)
			} else {
				// key := e.Latest().proxyKey
				// if _, err := s.proxyTranslator.xdsCache.GetSnapshot(key); err == nil {
				// 	s.proxyTranslator.xdsCache.ClearSnapshot(e.Latest().proxyKey)
				// }
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

// ReportQueue returns the queue that contains the latest status reports.
// It will be constantly updated to contain the merged status report for Kube Gateway status.
func (s *ProxySyncer) ReportQueue() utils.AsyncQueue[reports.ReportMap] {
	return s.reportQueue
}

// BackendPolicyReportQueue returns the queue that contains the latest status reports for all backend policies.
// It will be constantly updated to contain the merged status report for backend policies.
func (s *ProxySyncer) BackendPolicyReportQueue() utils.AsyncQueue[reports.ReportMap] {
	return s.backendPolicyReportQueue
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
