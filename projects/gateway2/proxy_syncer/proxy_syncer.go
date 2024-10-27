package proxy_syncer

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	rlexternal "github.com/solo-io/gloo/projects/gloo/api/external/solo/ratelimit"
	skrl "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	extauthkubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1/kube/apis/enterprise.gloo.solo.io/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	rlkubev1a1 "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/common"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	istiogvr "istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/avast/retry-go/v4"
	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/solo-io/gloo/pkg/utils/statsutils"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	gwplugins "github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/translatorutils"
	ggv2utils "github.com/solo-io/gloo/projects/gateway2/utils"
	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	kubeupstreams "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ProxySyncer is responsible for translating Kubernetes Gateway CRs into Gloo Proxies
// and syncing the proxyClient with the newly translated proxies.
type ProxySyncer struct {
	controllerName string
	writeNamespace string

	inputs          *GatewayInputChannels
	mgr             manager.Manager
	k8sGwExtensions extensions.K8sGatewayExtensions

	proxyTranslator ProxyTranslator
	istioClient     kube.Client

	pods krt.Collection[krtcollections.LocalityPod]

	proxyReconcileQueue ggv2utils.AsyncQueue[gloov1.ProxyList]

	statusReport krt.Singleton[report]
	xdsSnapshots krt.Collection[xdsSnapWrapper]
	proxyTrigger *krt.RecomputeTrigger

	waitForSync []cache.InformerSynced
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

// labels used to uniquely identify Proxies that are managed by the kube gateway controller
var kubeGatewayProxyLabels = map[string]string{
	// the proxy type key/value must stay in sync with the one defined in projects/gateway2/translator/gateway_translator.go
	utils.ProxyTypeKey: utils.GatewayApiProxyValue,
}

// NewProxySyncer returns an implementation of the ProxySyncer
// The provided GatewayInputChannels are used to trigger syncs.
func NewProxySyncer(
	ctx context.Context,
	settings krt.Singleton[glookubev1.Settings],
	controllerName string,
	writeNamespace string,
	inputs *GatewayInputChannels,
	mgr manager.Manager,
	client kube.Client,
	pods krt.Collection[krtcollections.LocalityPod],
	k8sGwExtensions extensions.K8sGatewayExtensions,
	translator setup.TranslatorFactory,
	xdsCache envoycache.SnapshotCache,
	syncerExtensions []syncer.TranslatorSyncerExtension,
	glooReporter reporter.StatusReporter,
	proxyReconcileQueue ggv2utils.AsyncQueue[gloov1.ProxyList],
) *ProxySyncer {

	return &ProxySyncer{
		controllerName:      controllerName,
		writeNamespace:      writeNamespace,
		inputs:              inputs,
		mgr:                 mgr,
		k8sGwExtensions:     k8sGwExtensions,
		proxyTranslator:     NewProxyTranslator(translator, xdsCache, settings, syncerExtensions, glooReporter),
		istioClient:         client,
		pods:                pods,
		proxyReconcileQueue: proxyReconcileQueue,
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

type xdsSnapWrapper struct {
	snap            *xds.EnvoySnapshot
	proxyKey        string
	proxyWithReport translatorutils.ProxyWithReports
	pluginRegistry  registry.PluginRegistry
	fullReports     reporter.ResourceReports
}

var _ krt.ResourceNamer = xdsSnapWrapper{}

func (p xdsSnapWrapper) Equals(in xdsSnapWrapper) bool {
	return p.snap.Equal(in.snap)
}
func (p xdsSnapWrapper) ResourceName() string {
	return p.proxyKey
}

type glooProxy struct {
	proxy *gloov1.Proxy
	// plugins used to generate this proxy
	pluginRegistry registry.PluginRegistry
	// the GWAPI reports generated for translation from a GW->Proxy
	// this contains status for the Gateway and referenced Routes
	reportMap reports.ReportMap
}

var _ krt.ResourceNamer = glooProxy{}

func (p glooProxy) Equals(in glooProxy) bool {
	if !proto.Equal(p.proxy, in.proxy) {
		return false
	}
	if !maps.Equal(p.reportMap.Gateways, in.reportMap.Gateways) {
		return false
	}
	if !maps.Equal(p.reportMap.Routes, in.reportMap.Routes) {
		return false
	}
	return true
}
func (p glooProxy) ResourceName() string {
	return xds.SnapshotCacheKey(p.proxy)
}

// UpstreamWrapper provides a keying function for Gloo's `v1.Upstream`
type UpstreamWrapper = krtcollections.ResourceWrapper[*gloov1.Upstream]

type report struct {
	reports.ReportMap
}

func (r report) ResourceName() string {
	return "report"
}

// do we really need this for a singleton?
func (r report) Equals(in report) bool {
	if !maps.Equal(r.ReportMap.Gateways, in.ReportMap.Gateways) {
		return false
	}
	if !maps.Equal(r.ReportMap.Routes, in.ReportMap.Routes) {
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

func (s *ProxySyncer) Init(ctx context.Context) error {
	ctx = contextutils.WithLogger(ctx, "k8s-gw-syncer")
	logger := contextutils.LoggerFrom(ctx)

	// TODO: handle cfgmap noisiness? (https://github.com/solo-io/gloo/blob/main/projects/gloo/pkg/api/converters/kube/artifact_converter.go#L31)
	configMapClient := kclient.New[*corev1.ConfigMap](s.istioClient)
	configMaps := krt.WrapClient(configMapClient, krt.WithName("ConfigMaps"))

	secretClient := kclient.New[*corev1.Secret](s.istioClient)
	secrets := krt.WrapClient(secretClient, krt.WithName("Secrets"))

	authConfigs := SetupCollectionDynamic[extauthkubev1.AuthConfig](
		ctx,
		s.istioClient,
		extauthkubev1.SchemeGroupVersion.WithResource("authconfigs"),
		krt.WithName("KubeAuthConfigs"),
	)

	rlConfigs := SetupCollectionDynamic[rlkubev1a1.RateLimitConfig](
		ctx,
		s.istioClient,
		rlkubev1a1.SchemeGroupVersion.WithResource("ratelimitconfigs"),
		krt.WithName("KubeRateLimitConfigs"),
	)

	upstreams := SetupCollectionDynamic[glookubev1.Upstream](
		ctx,
		s.istioClient,
		glookubev1.SchemeGroupVersion.WithResource("upstreams"),
		krt.WithName("KubeUpstreams"),
	)

	// helper collection to map from the runtime.Object Upstream representation to the gloov1.Upstream wrapper
	glooUpstreams := krt.NewCollection(upstreams, func(kctx krt.HandlerContext, u *glookubev1.Upstream) *UpstreamWrapper {
		glooUs := &u.Spec
		glooUs.Metadata = &core.Metadata{}
		glooUs.GetMetadata().Name = u.GetName()
		glooUs.GetMetadata().Namespace = u.GetNamespace()
		us := &UpstreamWrapper{Inner: glooUs}
		return us
	}, krt.WithName("GlooUpstreams"))

	serviceClient := kclient.New[*corev1.Service](s.istioClient)
	services := krt.WrapClient(serviceClient, krt.WithName("Services"))

	inMemUpstreams := krt.NewManyCollection(services, func(kctx krt.HandlerContext, svc *corev1.Service) []UpstreamWrapper {
		uss := []UpstreamWrapper{}
		for _, port := range svc.Spec.Ports {
			us := kubeupstreams.ServiceToUpstream(ctx, svc, port)
			uss = append(uss, UpstreamWrapper{Inner: us})
		}
		return uss
	}, krt.WithName("InMemoryUpstreams"))

	finalUpstreams := krt.JoinCollection([]krt.Collection[UpstreamWrapper]{glooUpstreams, inMemUpstreams})

	inputs := NewGlooK8sEndpointInputs(s.proxyTranslator.settings, s.istioClient, s.pods, services, finalUpstreams)

	glooEndpoints := NewGlooK8sEndpoints(ctx, inputs)
	clas := newEnvoyEndpoints(glooEndpoints)

	kubeGateways := SetupCollectionDynamic[gwv1.Gateway](
		ctx,
		s.istioClient,
		istiogvr.KubernetesGateway_v1,
		krt.WithName("KubeGateways"),
	)

	// alternatively we could start as not synced, and mark ready once ctrl-runtime caches are synced
	s.proxyTrigger = krt.NewRecomputeTrigger(true)

	glooProxies := krt.NewCollection(kubeGateways, func(kctx krt.HandlerContext, gw *gwv1.Gateway) *glooProxy {
		s.proxyTrigger.MarkDependant(kctx)
		proxy := s.buildProxy(ctx, gw)
		return proxy
	})
	s.xdsSnapshots = krt.NewCollection(glooProxies, func(kctx krt.HandlerContext, proxy glooProxy) *xdsSnapWrapper {
		// we are recomputing xds snapshots as proxies have changed, signal that we need to sync xds with these new snapshots
		xdsSnap := s.buildXdsSnapshot(
			ctx,
			kctx,
			logger,
			&proxy,
			configMaps,
			clas,
			secrets,
			finalUpstreams,
			authConfigs,
			rlConfigs,
		)
		return xdsSnap
	})

	// build ProxyList collection as glooProxies change
	proxiesToReconcile := krt.NewSingleton(func(kctx krt.HandlerContext) *proxyList {
		proxies := krt.Fetch(kctx, glooProxies)
		var l gloov1.ProxyList
		for _, p := range proxies {
			l = append(l, p.proxy)
		}
		return &proxyList{l}
	})
	// handler to reconcile ProxyList for in-memory proxy client
	proxiesToReconcile.Register(func(o krt.Event[proxyList]) {
		var l gloov1.ProxyList
		if o.Event != controllers.EventDelete {
			l = o.Latest().list
		}
		s.reconcileProxies(ctx, l)
	})

	// as proxies are created, they also contain a reportMap containing status for the Gateway and associated HTTPRoutes (really parentRefs)
	// here we will merge reports that are per-Proxy to a singleton Report used to persist to k8s on a timer
	s.statusReport = krt.NewSingleton(func(kctx krt.HandlerContext) *report {
		proxies := krt.Fetch(kctx, glooProxies)
		merged := reports.NewReportMap()
		for _, p := range proxies {
			// 1. merge GW Reports for all Proxies' status reports
			maps.Copy(merged.Gateways, p.reportMap.Gateways)

			// 2. merge parentRefs into RouteReports
			for rnn, rr := range p.reportMap.Routes {
				// if we haven't encountered this route, just copy it over completely
				old := merged.Routes[rnn]
				if old == nil {
					merged.Routes[rnn] = rr
					continue
				}
				// else, let's merge our parentRefs into the existing map
				// obsGen will stay as-is...
				maps.Copy(p.reportMap.Routes[rnn].Parents, rr.Parents)
			}
		}
		return &report{merged}
	})
	s.waitForSync = []cache.InformerSynced{
		authConfigs.Synced().HasSynced,
		rlConfigs.Synced().HasSynced,
		configMaps.Synced().HasSynced,
		secrets.Synced().HasSynced,
		services.Synced().HasSynced,
		inputs.Endpoints.Synced().HasSynced,
		inputs.Pods.Synced().HasSynced,
		inputs.Upstreams.Synced().HasSynced,
		glooEndpoints.Synced().HasSynced,
		clas.Synced().HasSynced,
		s.pods.Synced().HasSynced,
		upstreams.Synced().HasSynced,
		glooUpstreams.Synced().HasSynced,
		finalUpstreams.Synced().HasSynced,
		inMemUpstreams.Synced().HasSynced,
		kubeGateways.Synced().HasSynced,
		glooProxies.Synced().HasSynced,
		s.xdsSnapshots.Synced().HasSynced,
	}
	return nil
}

func (s *ProxySyncer) Start(ctx context.Context) error {
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("starting %s Proxy Syncer", s.controllerName)
	// latestReport will be constantly updated to contain the merged status report for Kube Gateway status
	// when timer ticks, we will use the state of the mergedReports at that point in time to sync the status to k8s
	var latestReport reports.ReportMap
	latestReport = reports.NewReportMap()
	s.statusReport.Register(func(o krt.Event[report]) {
		if o.Event == controllers.EventDelete {
			// TODO: handle garbage collection (see: https://github.com/solo-io/solo-projects/issues/7086)
			return
		}
		latestReport = o.Latest().ReportMap
	})
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
	timer := time.NewTicker(time.Second * 1)
	var needsProxyRecompute = false
	for {
		select {
		case <-ctx.Done():
			logger.Debug("context done, stopping proxy syncer")
			return nil
		case <-timer.C:
			if needsProxyRecompute {
				needsProxyRecompute = false
				s.proxyTrigger.TriggerRecomputation()
			}
			go func() {
				s.syncGatewayStatus(ctx, latestReport)
				s.syncRouteStatus(ctx, latestReport)
			}()
			go func() {
				// TODO: make sure this can't happen in parallel with itself from previous iteration
				// we probably will re-work status anyway..
				logger.Debug("syncing status plugins")
				snaps := s.xdsSnapshots.List()
				for _, snapWrap := range snaps {
					var proxiesWithReports []translatorutils.ProxyWithReports
					proxiesWithReports = append(proxiesWithReports, snapWrap.proxyWithReport)

					initStatusPlugins(ctx, proxiesWithReports, snapWrap.pluginRegistry)
				}
				for _, snapWrap := range snaps {
					err := s.proxyTranslator.syncXdsAndStatus(ctx, snapWrap.snap, snapWrap.proxyKey, snapWrap.fullReports)
					if err != nil {
						logger.Errorf("error while syncing proxy '%s': %s", snapWrap.proxyKey, err.Error())
					}

					var proxiesWithReports []translatorutils.ProxyWithReports
					proxiesWithReports = append(proxiesWithReports, snapWrap.proxyWithReport)
					applyStatusPlugins(ctx, proxiesWithReports, snapWrap.pluginRegistry)
				}
			}()
		case <-s.inputs.genericEvent.Next():
			// event from ctrl-rtime, signal that we need to recompute proxies on next tick
			// this will not be necessary once we switch the "front side" of translation to krt
			needsProxyRecompute = true
		}
	}
}

// buildProxy performs translation of a kube Gateway -> gloov1.Proxy (really a wrapper type)
func (s *ProxySyncer) buildProxy(ctx context.Context, gw *gwv1.Gateway) *glooProxy {
	stopwatch := statsutils.NewTranslatorStopWatch("ProxySyncer")
	stopwatch.Start()

	pluginRegistry := s.k8sGwExtensions.CreatePluginRegistry(ctx)
	rm := reports.NewReportMap()
	r := reports.NewReporter(&rm)

	var translatedGateways []gwplugins.TranslatedGateway
	gatewayTranslator := s.k8sGwExtensions.GetTranslator(ctx, gw, pluginRegistry)
	if gatewayTranslator == nil {
		contextutils.LoggerFrom(ctx).Errorf("no translator found for Gateway %s (gatewayClass %s)", gw.Name, gw.Spec.GatewayClassName)
		return nil
	}
	proxy := gatewayTranslator.TranslateProxy(ctx, gw, s.writeNamespace, r)
	if proxy == nil {
		return nil
	}
	translatedGateways = append(translatedGateways, gwplugins.TranslatedGateway{
		Gateway: *gw,
	})

	duration := stopwatch.Stop(ctx)
	contextutils.LoggerFrom(ctx).Debugf("translated proxy %s/%s in %s", proxy.GetMetadata().GetNamespace(), proxy.GetMetadata().GetName(), duration.String())

	// TODO: these are likely unnecessary and should be removed!
	applyPostTranslationPlugins(ctx, pluginRegistry, &gwplugins.PostTranslationContext{
		TranslatedGateways: translatedGateways,
	})

	return &glooProxy{
		proxy:          proxy,
		pluginRegistry: pluginRegistry,
		reportMap:      rm,
	}
}

func (s *ProxySyncer) buildXdsSnapshot(
	ctx context.Context,
	kctx krt.HandlerContext,
	logger *zap.SugaredLogger,
	proxy *glooProxy,
	kcm krt.Collection[*corev1.ConfigMap],
	kep krt.Collection[EndpointResources],
	ks krt.Collection[*corev1.Secret],
	kus krt.Collection[UpstreamWrapper],
	authConfigs krt.Collection[*extauthkubev1.AuthConfig],
	rlConfigs krt.Collection[*rlkubev1a1.RateLimitConfig],
) *xdsSnapWrapper {
	// TODO: add stopwatch with debug log
	cfgmaps := krt.Fetch(kctx, kcm)
	endpoints := krt.Fetch(kctx, kep)
	secrets := krt.Fetch(kctx, ks)
	upstreams := krt.Fetch(kctx, kus)
	authcfgs := krt.Fetch(kctx, authConfigs)
	krlcfgs := krt.Fetch(kctx, rlConfigs)
	// NOTE: the objects from all these Fetch(...) calls comes from client-go/istio client
	// which does NOT do a DeepCopy like the default ctrl-rtime cache does, so it is crucial to not modify
	// the objects retrieved; they are used to build our ApiSnapshot so as long as the various translator & plugins
	// don't modify them, we are safe.
	// see also: https://github.com/solo-io/solo-projects/issues/7080

	latestSnap := gloosnapshot.ApiSnapshot{}
	latestSnap.Proxies = gloov1.ProxyList{proxy.proxy}

	acfgs := make([]*extauthv1.AuthConfig, 0, len(authcfgs))
	for _, kac := range authcfgs {
		acfgs = append(acfgs, &kac.Spec)
	}
	latestSnap.AuthConfigs = acfgs

	rlcfgs := make([]*skrl.RateLimitConfig, 0, len(krlcfgs))
	for _, rlc := range krlcfgs {
		erlc := rlexternal.RateLimitConfig(*rlc.DeepCopy())  // DeepCopy to fix copylocks...?
		skrlc := skrl.RateLimitConfig{RateLimitConfig: erlc} // would have to DeepCopy again here...
		rlcfgs = append(rlcfgs, &skrlc)
	}
	latestSnap.Ratelimitconfigs = rlcfgs

	as := make([]*gloov1.Artifact, 0, len(cfgmaps))
	for _, u := range cfgmaps {
		a := kubeconverters.KubeConfigMapToArtifact(u)
		as = append(as, a)
	}
	latestSnap.Artifacts = as

	// secret client needed to use existing kube secret -> gloo secret converters
	// the only actually use is to do client.NewResource() to get a gloov1.Secret
	legacySecretClient := &kubesecret.ResourceClient{
		KubeCoreResourceClient: common.KubeCoreResourceClient{
			ResourceType: &gloov1.Secret{},
		},
	}

	// this must be a solo-kit based kube secret client, we accept a panic if not
	gs := make([]*gloov1.Secret, 0, len(secrets))
	for _, i := range secrets {
		secret, err := kubeconverters.GlooSecretConverterChain.FromKubeSecret(ctx, legacySecretClient, i)
		if err != nil {
			logger.Errorf(
				"error while trying to convert kube secret %s to gloo secret: %s",
				client.ObjectKeyFromObject(i).String(), err)
			continue
		}
		if secret == nil {
			continue
		}
		// this must be a gloov1 secret, we accept a panic if not
		glooSecret := secret.(*gloov1.Secret)
		gs = append(gs, glooSecret)
	}
	latestSnap.Secrets = gs

	gupstreams := make([]*gloov1.Upstream, 0, len(upstreams))
	for _, u := range upstreams {
		gupstreams = append(gupstreams, u.Inner)
	}
	latestSnap.Upstreams = gupstreams

	xdsSnapshot, reports, proxyReport := s.proxyTranslator.buildXdsSnapshot(kctx, ctx, proxy.proxy, &latestSnap)

	// TODO(Law): now we not able to merge reports after translation!

	// build ResourceReports struct containing only this Proxy
	r := make(reporter.ResourceReports)
	filteredReports := reports.FilterByKind("Proxy")
	r[proxy.proxy] = filteredReports[proxy.proxy]

	// build object used by status plugins
	proxyWithReport := translatorutils.ProxyWithReports{
		Proxy: proxy.proxy,
		Reports: translatorutils.TranslationReports{
			ProxyReport:     proxyReport,
			ResourceReports: r,
		},
	}

	// this must be an EnvoySnapshot, we accept a panic() if not
	envoySnap := xdsSnapshot.(*xds.EnvoySnapshot)
	endpointsProto := make([]envoycache.Resource, 0, len(endpoints))
	var endpointsVersion uint64
	for _, ep := range endpoints {
		endpointsProto = append(endpointsProto, ep.Endpoints)
		endpointsVersion ^= ep.EndpointsVersion
	}
	clustersVersion := envoySnap.Clusters.Version
	envoySnap.Endpoints = envoycache.NewResources(fmt.Sprintf("%v-%v", clustersVersion, endpointsVersion), endpointsProto)

	logger.Debugf("added endpoints to snapshot %s (%v listeners, %v clusters, %v routes, %v endpoints)",
		proxy.ResourceName(), len(envoySnap.Listeners.Items), len(envoySnap.Clusters.Items), len(envoySnap.Routes.Items), len(envoySnap.Endpoints.Items))
	out := xdsSnapWrapper{
		snap:            envoySnap,
		proxyKey:        proxy.ResourceName(),
		proxyWithReport: proxyWithReport,
		// propagate plugins
		pluginRegistry: proxy.pluginRegistry,
		fullReports:    reports,
	}
	return &out
}

// SetupCollectionDynamic uses the dynamic client to setup an informer for a resource
// and then uses an intermediate krt collection to type the unstructured resource.
// This is a temporary workaround until we update to the latest istio version and can
// uncomment the code below for registering types.
// HACK: we don't want to use this long term, but it's letting me push forward with deveopment
func SetupCollectionDynamic[T any](
	ctx context.Context,
	client kube.Client,
	gvr schema.GroupVersionResource,
	opts ...krt.CollectionOption,
) krt.Collection[*T] {
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("setting up dynamic collection for %s", gvr.String())
	delayedClient := kclient.NewDelayedInformer[*unstructured.Unstructured](client, gvr, kubetypes.DynamicInformer, kclient.Filter{})
	mapper := krt.WrapClient(delayedClient, opts...)
	return krt.NewCollection(mapper, func(krtctx krt.HandlerContext, i *unstructured.Unstructured) **T {
		var empty T
		out := &empty
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(i.UnstructuredContent(), out)
		if err != nil {
			logger.DPanic("failed converting unstructured into %T: %v", empty, i)
			return nil
		}
		return &out
	})
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
	stopwatch := statsutils.NewTranslatorStopWatch("HTTPRouteStatusSyncer")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)

	// Sometimes the List returns stale (cached) httproutes, causing the status update to fail
	// with "the object has been modified" errors. Therefore we try the status updates in a retry loop.
	err := retry.Do(func() error {
		for rnn, _ := range rm.Routes {
			route := gwv1.HTTPRoute{}
			err := s.mgr.GetClient().Get(ctx, rnn, &route)
			if err != nil {
				// log this at error level because this is not an expected error
				logger.Error(err)
				return err
			}
			if status := rm.BuildRouteStatus(ctx, route, s.controllerName); status != nil {
				if !isHTTPRouteStatusEqual(&route.Status, status) {
					route.Status = *status
					if err := s.mgr.GetClient().Status().Update(ctx, &route); err != nil {
						// log this as debug, since we will retry
						logger.Debugw("httproute status update attempt failed", "error", err,
							"httproute", fmt.Sprintf("%s.%s", route.GetNamespace(), route.GetName()))
						return err
					}
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
		logger.Errorw("all attempts failed at updating httproute statuses", "error", err)
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
		for gwnn, _ := range rm.Gateways {
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
func (s *ProxySyncer) reconcileProxies(ctx context.Context, proxyList gloov1.ProxyList) {
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

func isHTTPRouteStatusEqual(objA, objB *gwv1.HTTPRouteStatus) bool {
	return cmp.Equal(objA, objB, opts)
}
