package proxy_syncer

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	istiogvr "istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"

	"github.com/avast/retry-go/v4"
	deprecatedproto "github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/solo-io/gloo/pkg/utils/statsutils"
	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/irtranslator"
	gwplugins "github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/translatorutils"
	ggv2utils "github.com/solo-io/gloo/projects/gateway2/utils"
	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
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
	writeNamespace string

	initialSettings *glookubev1.Settings
	inputs          *GatewayInputChannels
	mgr             manager.Manager
	extensions      extensionsplug.Plugin

	proxyTranslator ProxyTranslator
	istioClient     kube.Client

	augmentedPods krt.Collection[krtcollections.LocalityPod]
	uniqueClients krt.Collection[krtcollections.UniqlyConnectedClient]

	proxyReconcileQueue ggv2utils.AsyncQueue[gloov1.ProxyList]

	statusReport            krt.Singleton[report]
	mostXdsSnapshots        krt.Collection[GatewayXdsResources]
	perclientSnapCollection krt.Collection[XdsSnapWrapper]
	proxiesToReconcile      krt.Singleton[proxyList]

	translator setup.TranslatorFactory

	waitForSync []cache.InformerSynced

	gwtranslator       extensionsplug.K8sGwTranslator
	irtranslator       *irtranslator.Translator
	upstreamTranslator *irtranslator.UpstreamTranslator
}

type GatewayXdsResources struct {
	types.NamespacedName

	Reports reports.ReportMap
	// Clusters are items in the CDS response payload.
	Clusters     []envoycache.Resource
	ClustersHash uint64

	// Routes are items in the RDS response payload.
	Routes envoycache.Resources

	// Listeners are items in the LDS response payload.
	Listeners envoycache.Resources
}

func (r GatewayXdsResources) ResourceName() string {
	return r.NamespacedName.String()
}
func (r GatewayXdsResources) Equals(in GatewayXdsResources) bool {
	return r.NamespacedName == in.NamespacedName && report{r.Reports}.Equals(in.Reports) && r.ClustersHash == in.ClustersHash &&
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
		Reports:      r,
		ClustersHash: ch,
		Clusters:     c,
		Routes:       sliceToResources(xdsSnap.Routes),
		Listeners:    sliceToResources(xdsSnap.Listeners),
	}
}

type GatewayInputChannels struct {
	genericEvent ggv2utils.AsyncQueue[struct{}]
}

func (x *GatewayInputChannels) Kick(ctx context.Context) {
	x.genericEvent.Enqueue(struct{}{})
}

func NewGatewayInputChannels() *GatewayInputChannels {
	return &GatewayInputChannels{
		genericEvent: ggv2utils.NewAsyncQueue[struct{}](),
	}
}

// NewProxySyncer returns an implementation of the ProxySyncer
// The provided GatewayInputChannels are used to trigger syncs.
func NewProxySyncer(
	ctx context.Context,
	initialSettings *glookubev1.Settings,
	settings krt.Singleton[glookubev1.Settings],
	controllerName string,
	writeNamespace string,
	inputs *GatewayInputChannels,
	mgr manager.Manager,
	client kube.Client,
	augmentedPods krt.Collection[krtcollections.LocalityPod],
	uniqueClients krt.Collection[krtcollections.UniqlyConnectedClient],
	extensions extensionsplug.Plugin,
	translator setup.TranslatorFactory,
	xdsCache envoycache.SnapshotCache,
	syncerExtensions []syncer.TranslatorSyncerExtension,
	glooReporter reporter.StatusReporter,
	proxyReconcileQueue ggv2utils.AsyncQueue[gloov1.ProxyList],
) *ProxySyncer {
	return &ProxySyncer{
		initialSettings:     initialSettings,
		controllerName:      controllerName,
		writeNamespace:      writeNamespace,
		inputs:              inputs,
		extensions:          extensions,
		mgr:                 mgr,
		proxyTranslator:     NewProxyTranslator(translator, xdsCache, settings, syncerExtensions, glooReporter),
		istioClient:         client,
		augmentedPods:       augmentedPods,
		uniqueClients:       uniqueClients,
		proxyReconcileQueue: proxyReconcileQueue,
		// we would want to instantiate the translator here, but
		// current we plugins do not assume they may be called concurrently, which could be the case
		// with individual object translation.
		// there for we instantiate a new translator each time during translation.
		// once we audit the plugins to be safe for concurrent use, we can instantiate the translator here.
		// this will also have the advantage, that the plugin life-cycle will outlive a single translation
		// so that they could own krt collections internally.
		translator: translator,
	}
}

type ProxyTranslator struct {
	translator       setup.TranslatorFactory
	settings         krt.Singleton[glookubev1.Settings]
	syncerExtensions []syncer.TranslatorSyncerExtension
	xdsCache         envoycache.SnapshotCache
	// used to no-op during extension syncing as we only do it to get reports
	noopSnapSetter syncer.SnapshotSetter
	// we need to report on upstreams/proxies that we are responsible for translating and syncing
	// so we use this reporter to do so; do we also need to report authconfigs and RLCs...?
	// TODO: consolidate this with the status reporter used in the plugins
	// TODO: copy the leader election stuff (and maybe leaderStartupAction whatever that is)
	glooReporter reporter.StatusReporter
}

func NewProxyTranslator(translator setup.TranslatorFactory,
	xdsCache envoycache.SnapshotCache,
	settings krt.Singleton[glookubev1.Settings],
	syncerExtensions []syncer.TranslatorSyncerExtension,
	glooReporter reporter.StatusReporter,
) ProxyTranslator {
	return ProxyTranslator{
		translator:       translator,
		xdsCache:         xdsCache,
		settings:         settings,
		syncerExtensions: syncerExtensions,
		noopSnapSetter:   &syncer.NoOpSnapshotSetter{},
		glooReporter:     glooReporter,
	}
}

type glooProxy struct {
	gateway *ir.GatewayIR
	// the GWAPI reports generated for translation from a GW->Proxy
	// this contains status for the Gateway and referenced Routes
	reportMap reports.ReportMap
}

type report struct {
	reports.ReportMap
}

// do we really need this for a singleton?
func (r report) Equals(in reports.ReportMap) bool {
	if !maps.Equal(r.ReportMap.Gateways, in.Gateways) {
		return false
	}
	if !maps.Equal(r.ReportMap.HTTPRoutes, in.HTTPRoutes) {
		return false
	}
	if !maps.Equal(r.ReportMap.TCPRoutes, in.TCPRoutes) {
		return false
	}
	return true
}

type proxyList struct {
	list gloov1.ProxyList
}

func (p proxyList) ResourceName() string {
	return "proxyList"
}

func (p proxyList) Equals(in proxyList) bool {
	sorted := p.list.Sort()
	sortedIn := in.list.Sort()
	return slices.EqualFunc(sorted, sortedIn, func(x, y *gloov1.Proxy) bool {
		return proto.Equal(x, y)
	})
}

func (s *ProxySyncer) Init(ctx context.Context, dbg *krt.DebugHandler) error {
	ctx = contextutils.WithLogger(ctx, "k8s-gw-proxy-syncer")
	logger := contextutils.LoggerFrom(ctx)
	withDebug := krt.WithDebugging(dbg)

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

	upstreams := krtutil.SetupCollectionDynamic[glookubev1.Upstream](
		ctx,
		s.istioClient,
		glookubev1.SchemeGroupVersion.WithResource("upstreams"),
		krt.WithName("KubeUpstreams"), withDebug,
	)

	// helper collection to map from the runtime.Object Upstream representation to the gloov1.Upstream wrapper
	glooUpstreams := krt.NewCollection(upstreams, func(kctx krt.HandlerContext, u *glookubev1.Upstream) *krtcollections.UpstreamWrapper {
		glooUs := &u.Spec
		md := core.Metadata{
			Name:      u.GetName(),
			Namespace: u.GetNamespace(),
		}
		glooUs.SetMetadata(&md)
		glooUs.NamespacedStatuses = &u.Status
		us := &krtcollections.UpstreamWrapper{Inner: glooUs}
		return us
	}, krt.WithName("GlooUpstreams"), withDebug)

	serviceClient := kclient.New[*corev1.Service](s.istioClient)
	services := krt.WrapClient(serviceClient, krt.WithName("Services"), withDebug)

	upstreamIndex := krtcollections.NewUpstreamIndex(nil)
	allEndpoints := []krt.Collection[krtcollections.EndpointsForUpstream]{}
	svcGk := schema.GroupKind{
		Group: corev1.GroupName,
		Kind:  "Service",
	}
	k8sServiceUpstreams := krtcollections.AddUpstreamMany[*corev1.Service](upstreamIndex, svcGk, services, func(kctx krt.HandlerContext, svc *corev1.Service) []ir.Upstream {
		uss := []ir.Upstream{}
		for _, port := range svc.Spec.Ports {
			uss = append(uss, ir.Upstream{
				ObjectSource: ir.ObjectSource{
					Kind:      corev1.GroupName,
					Group:     "Service",
					Namespace: svc.Namespace,
					Name:      svc.Name,
				},
				Obj:  svc,
				Port: port.Port,
				//TODO:zs				CanonicalHostname: ,
			})
		}
		return uss
	}, krt.WithName("KubernetesServiceUpstreams"), withDebug)

	inputs := krtcollections.NewGlooK8sEndpointInputs(s.proxyTranslator.settings, s.istioClient, dbg, s.augmentedPods, k8sServiceUpstreams)
	k8sServiceEndpoints := krtcollections.NewGlooK8sEndpoints(ctx, inputs)
	allEndpoints = append(allEndpoints, k8sServiceEndpoints)

	for k, col := range s.extensions.ContributesUpstreams {
		if col.Upstreams != nil {
			upstreamIndex.AddUpstreams(k, col.Upstreams)
		}
		if col.Endpoints != nil {
			allEndpoints = append(allEndpoints, col.Endpoints)
		}
	}

	finalUpstreams := krt.JoinCollection(upstreamIndex.Upstreams(), withDebug, krt.WithName("FinalUpstreams"))

	// build Endpoint intermediate representation from kubernetes service and extensions
	// TODO move kube service to be an extension
	endpointIRs := krt.JoinCollection(allEndpoints, withDebug, krt.WithName("EndpointIRs"))

	clas := newEnvoyEndpoints(endpointIRs, dbg)

	kubeRawGateways := krtutil.SetupCollectionDynamic[gwv1.Gateway](
		ctx,
		s.istioClient,
		istiogvr.KubernetesGateway_v1,
		krt.WithName("KubeGateways"), withDebug,
	)
	func() { panic("TODO: pass in policies") }() // wrap panic in a function to make vscode not see what's after it as dead code
	kubeGateways := krtcollections.NewGatweayIndex(nil, kubeRawGateways)

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
	}, withDebug, krt.WithName("MostXdsSnapshots"))
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

	epPerClient := NewPerClientEnvoyEndpoints(logger.Desugar(), dbg, s.uniqueClients, endpointIRs, endpointPlugins)
	clustersPerClient := NewPerClientEnvoyClusters(ctx, dbg, s.upstreamTranslator, finalUpstreams, s.uniqueClients)
	s.perclientSnapCollection = snapshotPerClient(logger.Desugar(), dbg, s.uniqueClients, s.mostXdsSnapshots, epPerClient, clustersPerClient)

	// as proxies are created, they also contain a reportMap containing status for the Gateway and associated xRoutes (really parentRefs)
	// here we will merge reports that are per-Proxy to a singleton Report used to persist to k8s on a timer
	s.statusReport = krt.NewSingleton(func(kctx krt.HandlerContext) *report {
		proxies := krt.Fetch(kctx, s.mostXdsSnapshots)
		merged := reports.NewReportMap()
		for _, p := range proxies {
			// 1. merge GW Reports for all Proxies' status reports
			maps.Copy(merged.Gateways, p.Reports.Gateways)

			// 2. merge httproute parentRefs into RouteReports
			for rnn, rr := range p.Reports.HTTPRoutes {
				// if we haven't encountered this route, just copy it over completely
				old := merged.HTTPRoutes[rnn]
				if old == nil {
					merged.HTTPRoutes[rnn] = rr
					continue
				}
				// else, let's merge our parentRefs into the existing map
				// obsGen will stay as-is...
				maps.Copy(p.Reports.HTTPRoutes[rnn].Parents, rr.Parents)
			}

			// 3. merge tcproute parentRefs into RouteReports
			for rnn, rr := range p.Reports.TCPRoutes {
				// if we haven't encountered this route, just copy it over completely
				old := merged.TCPRoutes[rnn]
				if old == nil {
					merged.TCPRoutes[rnn] = rr
					continue
				}
				// else, let's merge our parentRefs into the existing map
				// obsGen will stay as-is...
				maps.Copy(p.Reports.TCPRoutes[rnn].Parents, rr.Parents)
			}
		}
		return &report{merged}
	})

	s.waitForSync = []cache.InformerSynced{
		services.Synced().HasSynced,
		inputs.EndpointSlices.Synced().HasSynced,
		inputs.Pods.Synced().HasSynced,
		inputs.Upstreams.Synced().HasSynced,
		endpointIRs.Synced().HasSynced,
		clas.Synced().HasSynced,
		s.augmentedPods.Synced().HasSynced,
		upstreams.Synced().HasSynced,
		glooUpstreams.Synced().HasSynced,
		finalUpstreams.Synced().HasSynced,
		k8sServiceUpstreams.Synced().HasSynced,
		kubeGateways.Gateways.Synced().HasSynced,
		s.perclientSnapCollection.Synced().HasSynced,
		s.mostXdsSnapshots.Synced().HasSynced,
		s.extensions.HasSynced,
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
		latestReportQueue.Enqueue(o.Latest().ReportMap)
	})

	// handler to reconcile ProxyList for in-memory proxy client
	s.proxiesToReconcile.Register(func(o krt.Event[proxyList]) {
		var l gloov1.ProxyList
		if o.Event != controllers.EventDelete {
			l = o.Latest().list
		}
		s.reconcileProxies(l)
	})

	go func() {
		timer := time.NewTicker(time.Second * 1)
		for {
			select {
			case <-ctx.Done():
				logger.Debug("context done, stopping proxy syncer")
				return
			case <-timer.C:
				panic("TODO: implement status for plugins")
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
				s.proxyTranslator.syncXds(ctx, snapWrap.snap, snapWrap.proxyKey)
			} else {
				// key := e.Latest().proxyKey
				// if _, err := s.proxyTranslator.xdsCache.GetSnapshot(key); err == nil {
				// 	s.proxyTranslator.xdsCache.ClearSnapshot(e.Latest().proxyKey)
				// }
			}
		}
	}, true)

	go func() {
		timer := time.NewTicker(time.Second * 1)
		needsProxyRecompute := false
		for {
			select {
			case <-ctx.Done():
				logger.Debug("context done, stopping proxy recompute")
				return
			case <-timer.C:
				if needsProxyRecompute {
					needsProxyRecompute = false
					//s.proxyTrigger.TriggerRecomputation()
				}
			case <-s.inputs.genericEvent.Next():
				// event from ctrl-rtime, signal that we need to recompute proxies on next tick
				// this will not be necessary once we switch the "front side" of translation to krt
				needsProxyRecompute = true
			}
		}
	}()

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
		gatewayTranslator = s.extensions.ContributesGwTranslator(gw.Obj)
		if gatewayTranslator == nil {
			contextutils.LoggerFrom(ctx).Errorf("no translator found for Gateway %s (gatewayClass %s)", gw.Name, gw.Obj.Spec.GatewayClassName)
			return nil
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

func applyStatusPlugins(
	ctx context.Context,
	proxiesWithReports []translatorutils.ProxyWithReports,
	registry registry.PluginRegistry,
) {
	ctx = contextutils.WithLogger(ctx, "k8sGatewayStatusPlugins")
	logger := contextutils.LoggerFrom(ctx)

	statusCtx := &gwplugins.StatusContext{
		ProxiesWithReports: proxiesWithReports,
	}
	for _, plugin := range registry.GetStatusPlugins() {
		err := plugin.ApplyStatusPlugin(ctx, statusCtx)
		if err != nil {
			logger.Errorf("Error applying status plugin: %v", err)
			continue
		}
	}
}

func initStatusPlugins(
	ctx context.Context,
	proxiesWithReports []translatorutils.ProxyWithReports,
	registry registry.PluginRegistry,
) {
	ctx = contextutils.WithLogger(ctx, "k8sGatewayStatusPlugins")
	logger := contextutils.LoggerFrom(ctx)

	statusCtx := &gwplugins.StatusContext{
		ProxiesWithReports: proxiesWithReports,
	}
	for _, plugin := range registry.GetStatusPlugins() {
		err := plugin.InitStatusPlugin(ctx, statusCtx)
		if err != nil {
			logger.Errorf("Error applying init status plugin: %v", err)
		}
	}
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

// reconcileProxies persists the provided proxies by reconciling them with the proxyReconciler.
// as the Kube GW impl does not support reading Proxies from etcd, the expectation is these prox ies are
// written and persisted to the in-memory cache.
// The list MUST contain all valid kube Gw proxies, as the edge reconciler expects the full set; proxies that
// are not added to this list will be garbage collected by the solo-kit base reconciler, so this list must be the
// full SotW.
// The Gloo Xds translator_syncer will receive these proxies via List() using a MultiResourceClient.
// There are two reasons we must make these proxies available to legacy syncer:
// 1. To allow Rate Limit extensions to work, as it only syncs RL configs it finds used on Proxies in the snapshots
// 2. For debug tooling, notably the debug.ProxyEndpointServer
func (s *ProxySyncer) reconcileProxies(proxyList gloov1.ProxyList) {
	// gloo edge v1 will read from this queue
	s.proxyReconcileQueue.Enqueue(proxyList)
}

func applyPostTranslationPlugins(ctx context.Context, pluginRegistry registry.PluginRegistry, translationContext *gwplugins.PostTranslationContext) {
	ctx = contextutils.WithLogger(ctx, "postTranslation")
	logger := contextutils.LoggerFrom(ctx)

	for _, postTranslationPlugin := range pluginRegistry.GetPostTranslationPlugins() {
		err := postTranslationPlugin.ApplyPostTranslationPlugin(ctx, translationContext)
		if err != nil {
			logger.Errorf("Error applying post-translation plugin: %v", err)
			continue
		}
	}
}

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
