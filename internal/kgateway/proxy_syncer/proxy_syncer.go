package proxy_syncer

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"

	"github.com/avast/retry-go/v4"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/solo-io/go-utils/contextutils"
	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	extensions "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	ggv2utils "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/xds"
)

// ProxySyncer is responsible for translating Kubernetes Gateway CRs into Gloo Proxies
// and syncing the proxyClient with the newly translated proxies.
type ProxySyncer struct {
	controllerName string

	mgr              manager.Manager
	commonCols       *common.CommonCollections
	translatorSyncer *translator.CombinedTranslator
	extensions       extensionsplug.Plugin

	istioClient     kube.Client
	proxyTranslator ProxyTranslator

	uniqueClients krt.Collection[ir.UniqlyConnectedClient]

	statusReport            krt.Singleton[report]
	mostXdsSnapshots        krt.Collection[GatewayXdsResources]
	perclientSnapCollection krt.Collection[XdsSnapWrapper]

	waitForSync []cache.InformerSynced
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
	return r.NamespacedName == in.NamespacedName && report{r.reports}.Equals(report{in.reports}) && r.ClustersHash == in.ClustersHash &&
		r.Routes.Version == in.Routes.Version && r.Listeners.Version == in.Listeners.Version
}
func sliceToResourcesHash[T proto.Message](slice []T) ([]envoycachetypes.ResourceWithTTL, uint64) {
	var slicePb []envoycachetypes.ResourceWithTTL
	var resourcesHash uint64
	for _, r := range slice {
		var m proto.Message = r
		hash := ggv2utils.HashProto(r)
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

// NewProxySyncer returns an implementation of the ProxySyncer
// The provided GatewayInputChannels are used to trigger syncs.
func NewProxySyncer(
	ctx context.Context,
	controllerName string,
	mgr manager.Manager,
	client kube.Client,
	uniqueClients krt.Collection[ir.UniqlyConnectedClient],
	extensionsFactory extensions.K8sGatewayExtensionsFactory,
	commonCols *common.CommonCollections,
	xdsCache envoycache.SnapshotCache,
) *ProxySyncer {
	extensions := extensionsFactory(ctx, commonCols)

	return &ProxySyncer{
		controllerName:   controllerName,
		commonCols:       commonCols,
		mgr:              mgr,
		istioClient:      client,
		proxyTranslator:  NewProxyTranslator(xdsCache),
		uniqueClients:    uniqueClients,
		translatorSyncer: translator.NewCombinedTranslator(ctx, extensions, commonCols),
		extensions:       extensions,
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
	if !maps.Equal(r.reportMap.HTTPRoutes, in.reportMap.HTTPRoutes) {
		return false
	}
	if !maps.Equal(r.reportMap.TCPRoutes, in.reportMap.TCPRoutes) {
		return false
	}
	return true
}

// Note: isOurGw is shared between us and the deployer.
func (s *ProxySyncer) Init(ctx context.Context, isOurGw func(gw *gwv1.Gateway) bool, krtopts krtutil.KrtOptions) error {
	ctx = contextutils.WithLogger(ctx, "k8s-gw-proxy-syncer")
	logger := contextutils.LoggerFrom(ctx)

	kubeGateways, routes, upstreamIndex, endpointIRs := krtcollections.InitCollections(
		ctx,
		s.extensions,
		s.istioClient,
		isOurGw,
		s.commonCols.RefGrants,
		krtopts,
	)

	finalUpstreams := krt.JoinCollection(upstreamIndex.Upstreams(), krtopts.ToOptions("FinalUpstreams")...)

	// add the upstreams to the common collections, so they are available for policies.
	s.commonCols.Upstreams = upstreamIndex

	s.translatorSyncer.Init(ctx, routes)

	s.mostXdsSnapshots = krt.NewCollection(kubeGateways.Gateways, func(kctx krt.HandlerContext, gw ir.Gateway) *GatewayXdsResources {
		logger.Debugf("building proxy for kube gw %s version %s", client.ObjectKeyFromObject(gw.Obj), gw.Obj.GetResourceVersion())

		xdsSnap, rm := s.translatorSyncer.TranslateGateway(kctx, ctx, gw)
		if xdsSnap == nil {
			return nil
		}

		return toResources(gw, *xdsSnap, rm)
	}, krtopts.ToOptions("MostXdsSnapshots")...)

	epPerClient := NewPerClientEnvoyEndpoints(
		logger.Desugar(),
		krtopts,
		s.uniqueClients,
		endpointIRs,
		s.translatorSyncer.TranslateEndpoints,
	)
	clustersPerClient := NewPerClientEnvoyClusters(
		ctx,
		krtopts,
		s.translatorSyncer.GetUpstreamTranslator(),
		finalUpstreams,
		s.uniqueClients,
	)
	s.perclientSnapCollection = snapshotPerClient(
		logger.Desugar(),
		krtopts,
		s.uniqueClients,
		s.mostXdsSnapshots,
		epPerClient,
		clustersPerClient,
	)

	// as proxies are created, they also contain a reportMap containing status for the Gateway and associated xRoutes (really parentRefs)
	// here we will merge reports that are per-Proxy to a singleton Report used to persist to k8s on a timer
	s.statusReport = krt.NewSingleton(func(kctx krt.HandlerContext) *report {
		proxies := krt.Fetch(kctx, s.mostXdsSnapshots)
		merged := reports.NewReportMap()
		for _, p := range proxies {
			// 1. merge GW Reports for all Proxies' status reports
			maps.Copy(merged.Gateways, p.reports.Gateways)

			// 2. merge httproute parentRefs into RouteReports
			for rnn, rr := range p.reports.HTTPRoutes {
				// if we haven't encountered this route, just copy it over completely
				old := merged.HTTPRoutes[rnn]
				if old == nil {
					merged.HTTPRoutes[rnn] = rr
					continue
				}
				// else, let's merge our parentRefs into the existing map
				// obsGen will stay as-is...
				maps.Copy(p.reports.HTTPRoutes[rnn].Parents, rr.Parents)
			}

			// 3. merge tcproute parentRefs into RouteReports
			for rnn, rr := range p.reports.TCPRoutes {
				// if we haven't encountered this route, just copy it over completely
				old := merged.TCPRoutes[rnn]
				if old == nil {
					merged.TCPRoutes[rnn] = rr
					continue
				}
				// else, let's merge our parentRefs into the existing map
				// obsGen will stay as-is...
				maps.Copy(p.reports.TCPRoutes[rnn].Parents, rr.Parents)
			}
		}
		return &report{merged}
	})

	s.waitForSync = []cache.InformerSynced{
		endpointIRs.Synced().HasSynced,
		endpointIRs.Synced().HasSynced,
		upstreamIndex.HasSynced,
		finalUpstreams.Synced().HasSynced,
		kubeGateways.Gateways.Synced().HasSynced,
		s.perclientSnapCollection.Synced().HasSynced,
		s.mostXdsSnapshots.Synced().HasSynced,
		s.extensions.HasSynced,
		routes.HasSynced,
		s.translatorSyncer.HasSynced,
	}
	return nil
}

func (s *ProxySyncer) Start(ctx context.Context) error {
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("starting %s Proxy Syncer", s.controllerName)
	// latestReport will be constantly updated to contain the merged status report for Kube Gateway status
	// when timer ticks, we will use the state of the mergedReports at that point in time to sync the status to k8s
	latestReportQueue := ggv2utils.NewAsyncQueue[reports.ReportMap]()
	logger.Infof("waiting for cache to sync")

	// wait for krt collections to sync
	s.istioClient.WaitForCacheSync(
		"kube gw proxy syncer",
		ctx.Done(),
		s.waitForSync...,
	)

	// wait for ctrl-rtime caches to sync before accepting events
	if !s.mgr.GetCache().WaitForCacheSync(ctx) {
		return errors.New("kube gateway sync loop waiting for all caches to sync failed")
	}

	logger.Infof("caches warm!")

	// caches are warm, now we can do registrations
	s.statusReport.Register(func(o krt.Event[report]) {
		if o.Event == controllers.EventDelete {
			// TODO: handle garbage collection (see: https://github.com/solo-io/solo-projects/issues/7086)
			return
		}
		latestReportQueue.Enqueue(o.Latest().reportMap)
	})

	go func() {
		timer := time.NewTicker(time.Second * 1)
		for {
			select {
			case <-ctx.Done():
				logger.Debug("context done, stopping proxy syncer")
				return
			case <-timer.C:
				//				panic("TODO: implement status for plugins")
				/*
					snaps := s.mostXdsSnapshots.List()
					for _, snapWrap := range snaps {
						var proxiesWithReports []translatorutils.ProxyWithReports
						proxiesWithReports = append(proxiesWithReports, snapWrap.Reports)

						initStatusPlugins(ctx, proxiesWithReports, snapWrap.pluginRegistry)
					}
					for _, snapWrap := range snaps {
						err := s.proxyTranslator.syncStatus(ctx, snapWrap.proxyKey, snapWrap.fullReports)
						if err != nil {
							logger.Errorf("error while syncing proxy '%s': %s", snapWrap.proxyKey, err.Error())
						}

						var proxiesWithReports []translatorutils.ProxyWithReports
						proxiesWithReports = append(proxiesWithReports, snapWrap.proxyWithReport)
						applyStatusPlugins(ctx, proxiesWithReports, snapWrap.pluginRegistry)
					}
				*/
			}
		}
	}()

	s.perclientSnapCollection.RegisterBatch(func(o []krt.Event[XdsSnapWrapper], initialSync bool) {
		for _, e := range o {
			if e.Event != controllers.EventDelete {
				snapWrap := e.Latest()
				s.proxyTranslator.syncXds(ctx, snapWrap)
			} else {
				// key := e.Latest().proxyKey
				// if _, err := s.proxyTranslator.xdsCache.GetSnapshot(key); err == nil {
				// 	s.proxyTranslator.xdsCache.ClearSnapshot(e.Latest().proxyKey)
				// }
			}
		}
	}, true)

	go func() {
		for {
			latestReport, err := latestReportQueue.Dequeue(ctx)
			if err != nil {
				return
			}
			s.syncGatewayStatus(ctx, latestReport)
			s.syncRouteStatus(ctx, latestReport)
		}
	}()
	<-ctx.Done()
	return nil
}

func (s *ProxySyncer) syncRouteStatus(ctx context.Context, rm reports.ReportMap) {
	ctx = contextutils.WithLogger(ctx, "routeStatusSyncer")
	logger := contextutils.LoggerFrom(ctx)
	stopwatch := utils.NewTranslatorStopWatch("RouteStatusSyncer")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)

	// Helper function to sync route status with retry
	syncStatusWithRetry := func(routeType string, routeKey client.ObjectKey, getRouteFunc func() client.Object, statusUpdater func(route client.Object) error) error {
		return retry.Do(func() error {
			route := getRouteFunc()
			err := s.mgr.GetClient().Get(ctx, routeKey, route)
			if err != nil {
				logger.Errorw(fmt.Sprintf("%s get failed", routeType), "error", err, "route", routeKey)
				return err
			}
			if err := statusUpdater(route); err != nil {
				logger.Debugw(fmt.Sprintf("%s status update attempt failed", routeType), "error", err,
					"route", fmt.Sprintf("%s.%s", routeKey.Namespace, routeKey.Name))
				return err
			}
			return nil
		},
			retry.Attempts(5),
			retry.Delay(100*time.Millisecond),
			retry.DelayType(retry.BackOffDelay),
		)
	}

	// Helper function to build route status and update if needed
	buildAndUpdateStatus := func(route client.Object, routeType string) error {
		var status *gwv1.RouteStatus

		switch r := route.(type) {
		case *gwv1.HTTPRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil
			}
			r.Status.RouteStatus = *status
		case *gwv1a2.TCPRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil
			}
			r.Status.RouteStatus = *status
		default:
			logger.Warnw(fmt.Sprintf("unsupported route type for %s", routeType), "route", route)
			return nil
		}

		// Update the status
		return s.mgr.GetClient().Status().Update(ctx, route)
	}

	// Sync HTTPRoute statuses
	for rnn := range rm.HTTPRoutes {
		err := syncStatusWithRetry(wellknown.HTTPRouteKind, rnn, func() client.Object { return new(gwv1.HTTPRoute) }, func(route client.Object) error {
			return buildAndUpdateStatus(route, wellknown.HTTPRouteKind)
		})
		if err != nil {
			logger.Errorw("all attempts failed at updating HTTPRoute status", "error", err, "route", rnn)
		}
	}

	// Sync TCPRoute statuses
	for rnn := range rm.TCPRoutes {
		err := syncStatusWithRetry(wellknown.TCPRouteKind, rnn, func() client.Object { return new(gwv1a2.TCPRoute) }, func(route client.Object) error {
			return buildAndUpdateStatus(route, wellknown.TCPRouteKind)
		})
		if err != nil {
			logger.Errorw("all attempts failed at updating TCPRoute status", "error", err, "route", rnn)
		}
	}
}

// syncGatewayStatus will build and update status for all Gateways in a reportMap
func (s *ProxySyncer) syncGatewayStatus(ctx context.Context, rm reports.ReportMap) {
	ctx = contextutils.WithLogger(ctx, "statusSyncer")
	logger := contextutils.LoggerFrom(ctx)
	stopwatch := utils.NewTranslatorStopWatch("GatewayStatusSyncer")
	stopwatch.Start()

	// TODO: retry within loop per GW rathen that as a full block
	err := retry.Do(func() error {
		for gwnn := range rm.Gateways {
			gw := gwv1.Gateway{}
			err := s.mgr.GetClient().Get(ctx, gwnn, &gw)
			if err != nil {
				logger.Info("error getting gw", err.Error())
				return err
			}
			gwStatusWithoutAddress := gw.Status
			gwStatusWithoutAddress.Addresses = nil
			if status := rm.BuildGWStatus(ctx, gw); status != nil {
				if !isGatewayStatusEqual(&gwStatusWithoutAddress, status) {
					gw.Status = *status
					if err := s.mgr.GetClient().Status().Patch(ctx, &gw, client.Merge); err != nil {
						logger.Error(err)
						return err
					}
					logger.Infof("patched gw '%s' status", gwnn.String())
				} else {
					logger.Infof("skipping k8s gateway %s status update, status equal", gwnn.String())
				}
			}
		}
		return nil
	},
		retry.Attempts(5),
		retry.Delay(100*time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		logger.Errorw("all attempts failed at updating gateway statuses", "error", err)
	}
	duration := stopwatch.Stop(ctx)
	logger.Debugf("synced gw status for %d gateways in %s", len(rm.Gateways), duration.String())
}

//func applyPostTranslationPlugins(ctx context.Context, pluginRegistry registry.PluginRegistry, translationContext *gwplugins.PostTranslationContext) {
//	ctx = contextutils.WithLogger(ctx, "postTranslation")
//	logger := contextutils.LoggerFrom(ctx)
//
//	for _, postTranslationPlugin := range pluginRegistry.GetPostTranslationPlugins() {
//		err := postTranslationPlugin.ApplyPostTranslationPlugin(ctx, translationContext)
//		if err != nil {
//			logger.Errorf("Error applying post-translation plugin: %v", err)
//			continue
//		}
//	}
//}

var opts = cmp.Options{
	cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
	cmpopts.IgnoreMapEntries(func(k string, _ any) bool {
		return k == "lastTransitionTime"
	}),
}

func isGatewayStatusEqual(objA, objB *gwv1.GatewayStatus) bool {
	return cmp.Equal(objA, objB, opts)
}

// isRouteStatusEqual compares two RouteStatus objects directly
func isRouteStatusEqual(objA, objB *gwv1.RouteStatus) bool {
	return cmp.Equal(objA, objB, opts)
}

type resourcesStringer envoycache.Resources

func (r resourcesStringer) String() string {
	return fmt.Sprintf("len: %d, version %s", len(r.Items), r.Version)
}
