package proxy_syncer

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"

	"github.com/avast/retry-go/v4"
	deprecatedproto "github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/solo-io/gloo/pkg/utils/statsutils"
	extensions "github.com/solo-io/gloo/projects/gateway2/extensions2"
	"github.com/solo-io/gloo/projects/gateway2/extensions2/common"
	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator"
	"github.com/solo-io/gloo/projects/gateway2/translator/irtranslator"
	ggv2utils "github.com/solo-io/gloo/projects/gateway2/utils"
	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const gatewayV1A2Version = "v1alpha2"

// ProxySyncer is responsible for translating Kubernetes Gateway CRs into Gloo Proxies
// and syncing the proxyClient with the newly translated proxies.
type ProxySyncer struct {
	controllerName string

	initialSettings   *glookubev1.Settings
	settings          krt.Singleton[glookubev1.Settings]
	mgr               manager.Manager
	extensionsFactory extensions.K8sGatewayExtensionsFactory
	extensions        extensionsplug.Plugin
	commonCols        common.CommonCollections

	istioClient     kube.Client
	proxyTranslator ProxyTranslator

	augmentedPods krt.Collection[krtcollections.LocalityPod]
	uniqueClients krt.Collection[ir.UniqlyConnectedClient]

	statusReport            krt.Singleton[report]
	mostXdsSnapshots        krt.Collection[GatewayXdsResources]
	perclientSnapCollection krt.Collection[XdsSnapWrapper]

	waitForSync []cache.InformerSynced

	gwtranslator       extensionsplug.K8sGwTranslator
	irtranslator       *irtranslator.Translator
	upstreamTranslator *irtranslator.UpstreamTranslator
}

type GatewayXdsResources struct {
	types.NamespacedName

	reports reports.ReportMap
	// Clusters are items in the CDS response payload.
	Clusters     []envoycache.Resource
	ClustersHash uint64

	// Routes are items in the RDS response payload.
	Routes envoycache.Resources

	// Listeners are items in the LDS response payload.
	Listeners envoycache.Resources
}

func (r GatewayXdsResources) ResourceName() string {
	return xds.OwnerNamespaceNameID(glooutils.GatewayApiProxyValue, r.Namespace, r.Name)
}
func (r GatewayXdsResources) Equals(in GatewayXdsResources) bool {
	return r.NamespacedName == in.NamespacedName && report{r.reports}.Equals(report{in.reports}) && r.ClustersHash == in.ClustersHash &&
		r.Routes.Version == in.Routes.Version && r.Listeners.Version == in.Listeners.Version
}
func sliceToResourcesHash[T proto.Message](slice []T) ([]envoycache.Resource, uint64) {
	var slicePb []envoycache.Resource
	var resourcesHash uint64
	for _, r := range slice {
		var m proto.Message = r
		dm := m.(deprecatedproto.Message)
		hash := ggv2utils.HashProto(r)
		slicePb = append(slicePb, resource.NewEnvoyResource(envoycache.ResourceProto(dm)))
		resourcesHash ^= hash
	}

	return slicePb, resourcesHash
}

func sliceToResources[T proto.Message](slice []T) envoycache.Resources {
	r, h := sliceToResourcesHash(slice)
	return envoycache.NewResources(fmt.Sprintf("%d", h), r)

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
	initialSettings *glookubev1.Settings,
	settings krt.Singleton[glookubev1.Settings],
	controllerName string,
	mgr manager.Manager,
	client kube.Client,
	augmentedPods krt.Collection[krtcollections.LocalityPod],
	uniqueClients krt.Collection[ir.UniqlyConnectedClient],
	extensionsFactory extensions.K8sGatewayExtensionsFactory,
	commoncol common.CommonCollections,
	xdsCache envoycache.SnapshotCache,
) *ProxySyncer {
	return &ProxySyncer{
		initialSettings:   initialSettings,
		settings:          settings,
		controllerName:    controllerName,
		extensionsFactory: extensionsFactory,
		commonCols:        commoncol,
		mgr:               mgr,
		istioClient:       client,
		proxyTranslator:   NewProxyTranslator(xdsCache),
		augmentedPods:     augmentedPods,
		uniqueClients:     uniqueClients,
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

type glooProxy struct {
	gateway *ir.GatewayIR
	// the GWAPI reports generated for translation from a GW->Proxy
	// this contains status for the Gateway and referenced Routes
	reportMap reports.ReportMap
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

func (s *ProxySyncer) Init(ctx context.Context, isOurGw func(gw *gwv1.Gateway) bool, krtopts krtutil.KrtOptions) error {
	ctx = contextutils.WithLogger(ctx, "k8s-gw-proxy-syncer")
	logger := contextutils.LoggerFrom(ctx)

	s.extensions = s.extensionsFactory(ctx, &s.commonCols)

	nsCol := krtcollections.NewNamespaceCollection(ctx, s.istioClient, krtopts)

	kubeGateways, routes, finalUpstreams, endpointIRs := krtcollections.InitCollections(ctx, s.extensions, s.istioClient, isOurGw, s.commonCols.RefGrants, krtopts)
	queries := query.NewData(
		routes,
		s.commonCols.Secrets,
		nsCol,
	)
	s.gwtranslator = translator.NewTranslator(queries)
	s.irtranslator = &irtranslator.Translator{
		ContributedPolicies: s.extensions.ContributesPolicies,
	}
	s.upstreamTranslator = &irtranslator.UpstreamTranslator{
		ContributedUpstreams: make(map[schema.GroupKind]ir.UpstreamInit),
		ContributedPolicies:  s.extensions.ContributesPolicies,
	}
	for k, up := range s.extensions.ContributesUpstreams {
		s.upstreamTranslator.ContributedUpstreams[k] = up.UpstreamInit
	}

	s.mostXdsSnapshots = krt.NewCollection(kubeGateways.Gateways, func(kctx krt.HandlerContext, gw ir.Gateway) *GatewayXdsResources {
		logger.Debugf("building proxy for kube gw %s version %s", client.ObjectKeyFromObject(gw.Obj), gw.Obj.GetResourceVersion())
		rm := reports.NewReportMap()
		r := reports.NewReporter(&rm)
		gwir := s.buildProxy(kctx, ctx, gw, r)

		if gwir == nil {
			return nil
		}

		// we are recomputing xds snapshots as proxies have changed, signal that we need to sync xds with these new snapshots
		xdsSnap := s.irtranslator.Translate(*gwir, r)

		return toResources(gw, xdsSnap, rm)
	}, krtopts.ToOptions("MostXdsSnapshots")...)
	// TODO: disable dest rule plugin if we have setting
	//	if s.initialSettings.Spec.GetGloo().GetIstioOptions().GetEnableIntegration().GetValue() {
	//		s.destRules = NewDestRuleIndex(s.istioClient, dbg)
	//	} else {
	//		s.destRules = NewEmptyDestRuleIndex()
	//	}

	var endpointPlugins []extensionsplug.EndpointPlugin
	for _, ext := range s.extensions.ContributesPolicies {
		if ext.PerClientProcessEndpoints != nil {
			endpointPlugins = append(endpointPlugins, ext.PerClientProcessEndpoints)
		}
	}

	epPerClient := NewPerClientEnvoyEndpoints(logger.Desugar(), krtopts, s.uniqueClients, endpointIRs, endpointPlugins)
	clustersPerClient := NewPerClientEnvoyClusters(ctx, krtopts, s.upstreamTranslator, finalUpstreams, s.uniqueClients)
	s.perclientSnapCollection = snapshotPerClient(logger.Desugar(), krtopts, s.uniqueClients, s.mostXdsSnapshots, epPerClient, clustersPerClient)

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
		s.augmentedPods.Synced().HasSynced,
		finalUpstreams.Synced().HasSynced,
		kubeGateways.Gateways.Synced().HasSynced,
		s.perclientSnapCollection.Synced().HasSynced,
		s.mostXdsSnapshots.Synced().HasSynced,
		s.extensions.HasSynced,
		s.settings.AsCollection().Synced().HasSynced,
		routes.HasSynced,
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

// buildProxy performs translation of a kube Gateway -> gloov1.Proxy (really a wrapper type)
func (s *ProxySyncer) buildProxy(kctx krt.HandlerContext, ctx context.Context, gw ir.Gateway, r reports.Reporter) *ir.GatewayIR {
	stopwatch := statsutils.NewTranslatorStopWatch("ProxySyncer")
	stopwatch.Start()
	var gatewayTranslator extensionsplug.K8sGwTranslator = s.gwtranslator
	if s.extensions.ContributesGwTranslator != nil {
		maybeGatewayTranslator := s.extensions.ContributesGwTranslator(gw.Obj)
		if maybeGatewayTranslator != nil {
			// TODO: need better error handling here
			// and filtering out of our gateway classes, like before
			// contextutils.LoggerFrom(ctx).Errorf("no translator found for Gateway %s (gatewayClass %s)", gw.Name, gw.Obj.Spec.GatewayClassName)
			gatewayTranslator = maybeGatewayTranslator
		}
	} else {

	}
	proxy := gatewayTranslator.Translate(kctx, ctx, &gw, r)
	if proxy == nil {
		return nil
	}

	duration := stopwatch.Stop(ctx)
	contextutils.LoggerFrom(ctx).Debugf("translated proxy %s/%s in %s", gw.Namespace, gw.Name, duration.String())

	// TODO: these are likely unnecessary and should be removed!
	//	applyPostTranslationPlugins(ctx, pluginRegistry, &gwplugins.PostTranslationContext{
	//		TranslatedGateways: translatedGateways,
	//	})

	return proxy
}

func (s *ProxySyncer) syncRouteStatus(ctx context.Context, rm reports.ReportMap) {
	ctx = contextutils.WithLogger(ctx, "routeStatusSyncer")
	logger := contextutils.LoggerFrom(ctx)
	stopwatch := statsutils.NewTranslatorStopWatch("RouteStatusSyncer")
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
	stopwatch := statsutils.NewTranslatorStopWatch("GatewayStatusSyncer")
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
