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

	rlexternal "github.com/solo-io/gloo/projects/gloo/api/external/solo/ratelimit"
	skrl "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	extauthkubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1/kube/apis/enterprise.gloo.solo.io/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	rlkubev1a1 "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"

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
	"github.com/solo-io/gloo/projects/gateway2/reports"
	gwplugins "github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/translatorutils"
	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	kubeupstreams "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"

	"github.com/solo-io/solo-kit/pkg/utils/statusutils"
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

	// proxyReconciler wraps the client that writes Proxy resources into an in-memory cache
	// This cache is utilized by RateLimit and the debug.ProxyEndpointServer
	proxyReconciler gloov1.ProxyReconciler

	proxyTranslator ProxyTranslator
	istioClient     kube.Client

	// secret client needed to use existing kube secret -> gloo secret converters
	// the only actually use is to do client.NewResource() to get a gloov1.Secret
	// we can/should probably break this dependency entirely relatively easily
	legacySecretClient gloov1.SecretClient
}

type GatewayInputChannels struct {
	genericEvent AsyncQueue[struct{}]
}

func (x *GatewayInputChannels) Kick(ctx context.Context) {
	x.genericEvent.Enqueue(struct{}{})
}

func NewGatewayInputChannels() *GatewayInputChannels {
	return &GatewayInputChannels{
		genericEvent: NewAsyncQueue[struct{}](),
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
	controllerName string,
	writeNamespace string,
	inputs *GatewayInputChannels,
	mgr manager.Manager,
	k8sGwExtensions extensions.K8sGatewayExtensions,
	proxyClient gloov1.ProxyClient,
	translator translator.Translator,
	xdsCache envoycache.SnapshotCache,
	settings *gloov1.Settings,
	syncerExtensions []syncer.TranslatorSyncerExtension,
	legacySecretClient gloov1.SecretClient,
	glooReporter reporter.StatusReporter,
) *ProxySyncer {
	restCfg := kube.NewClientConfigForRestConfig(mgr.GetConfig())
	client, err := kube.NewClient(restCfg, "")
	if err != nil {
		// TODO: the istio kube client creation will be moved earlier in the flow in a follow-up,
		// so this will be able to be handled appropriately shortly
		panic(err)
	}
	kube.EnableCrdWatcher(client)

	return &ProxySyncer{
		controllerName:     controllerName,
		writeNamespace:     writeNamespace,
		inputs:             inputs,
		mgr:                mgr,
		k8sGwExtensions:    k8sGwExtensions,
		proxyReconciler:    gloov1.NewProxyReconciler(proxyClient, statusutils.NewNoOpStatusClient()),
		proxyTranslator:    NewProxyTranslator(translator, xdsCache, settings, syncerExtensions, glooReporter),
		istioClient:        client,
		legacySecretClient: legacySecretClient,
	}
}

type ProxyTranslator struct {
	translator       translator.Translator
	settings         *gloov1.Settings
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

func NewProxyTranslator(translator translator.Translator,
	xdsCache envoycache.SnapshotCache,
	settings *gloov1.Settings,
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

var _ krt.ResourceNamer = &glooEndpoint{}

// stolen from projects/gloo/pkg/upstreams/serviceentry/krtwrappers.go
func UnwrapEps(geps []*glooEndpoint) gloov1.EndpointList {
	out := make(gloov1.EndpointList, 0, len(geps))
	for _, ep := range geps {
		out = append(out, ep.Endpoint)
	}
	return out
}

// glooEndpoint provides a krt keying function for Gloo's `v1.Endpoint`
type glooEndpoint struct {
	*gloov1.Endpoint
}

func (ep *glooEndpoint) Equals(in *glooEndpoint) bool {
	return proto.Equal(ep, in)
}

func (ep *glooEndpoint) ResourceName() string {
	return ep.Metadata.GetName() + "/" + ep.Metadata.GetNamespace()
}

var _ krt.ResourceNamer = upstream{}

// upstream provides a keying function for Gloo's `v1.Upstream`
type upstream struct {
	*gloov1.Upstream
}

func (us upstream) ResourceName() string {
	return us.Metadata.GetName() + "/" + us.Metadata.GetNamespace()
}
func (us upstream) Equals(in upstream) bool {
	return proto.Equal(us, in)
}

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

func (s *ProxySyncer) Start(ctx context.Context) error {
	ctx = contextutils.WithLogger(ctx, "k8s-gw-syncer")
	logger := contextutils.LoggerFrom(ctx)

	// TODO: handle cfgmap noisiness? (https://github.com/solo-io/gloo/blob/main/projects/gloo/pkg/api/converters/kube/artifact_converter.go#L31)
	configMapClient := kclient.New[*corev1.ConfigMap](s.istioClient)
	configMaps := krt.WrapClient(configMapClient, krt.WithName("ConfigMaps"))

	secretClient := kclient.New[*corev1.Secret](s.istioClient)
	secrets := krt.WrapClient(secretClient, krt.WithName("Secrets"))

	authConfigs := setupCollectionDynamic[extauthkubev1.AuthConfig](
		ctx,
		s.istioClient,
		extauthkubev1.SchemeGroupVersion.WithResource("authconfigs"),
		krt.WithName("KubeAuthConfigs"),
	)

	rlConfigs := setupCollectionDynamic[rlkubev1a1.RateLimitConfig](
		ctx,
		s.istioClient,
		rlkubev1a1.SchemeGroupVersion.WithResource("ratelimitconfigs"),
		krt.WithName("KubeRateLimitConfigs"),
	)

	upstreams := setupCollectionDynamic[glookubev1.Upstream](
		ctx,
		s.istioClient,
		glookubev1.SchemeGroupVersion.WithResource("upstreams"),
		krt.WithName("KubeUpstreams"),
	)

	// helper collection to map from the runtime.Object Upstream representation to the gloov1.Upstream wrapper
	glooUpstreams := krt.NewCollection(upstreams, func(kctx krt.HandlerContext, u *glookubev1.Upstream) *upstream {
		glooUs := &u.Spec
		glooUs.Metadata = &core.Metadata{}
		glooUs.GetMetadata().Name = u.GetName()
		glooUs.GetMetadata().Namespace = u.GetNamespace()
		us := &upstream{glooUs}
		return us
	}, krt.WithName("GlooUpstreams"))

	serviceClient := kclient.New[*corev1.Service](s.istioClient)
	services := krt.WrapClient(serviceClient, krt.WithName("Services"))

	inMemUpstreams := krt.NewManyCollection(services, func(kctx krt.HandlerContext, svc *corev1.Service) []upstream {
		uss := []upstream{}
		for _, port := range svc.Spec.Ports {
			us := kubeupstreams.ServiceToUpstream(ctx, svc, port)
			uss = append(uss, upstream{us})
		}
		return uss
	}, krt.WithName("InMemoryUpstreams"))

	finalUpstreams := krt.JoinCollection([]krt.Collection[upstream]{glooUpstreams, inMemUpstreams})

	podClient := kclient.NewFiltered[*corev1.Pod](s.istioClient, kclient.Filter{
		ObjectTransform: kube.StripPodUnusedFields,
	})
	pods := krt.WrapClient(podClient, krt.WithName("Pods"))

	epClient := kclient.New[*corev1.Endpoints](s.istioClient)
	kubeEndpoints := krt.WrapClient(epClient, krt.WithName("Endpoints"))

	glooEndpoints := krt.NewManyFromNothing(func(kctx krt.HandlerContext) []*glooEndpoint {
		logger.Info("in gloo endpoints transformation")
		// NOTE: buildEndpoints(...) effectively duplicates the existing GE endpoint logic
		// into a KRT collection; this will be refactored entirely in a follow up from Yuval
		return buildEndpoints(kctx, logger, finalUpstreams, kubeEndpoints, services, pods)
	}, krt.WithName("GlooEndpoints"))

	kubeGateways := setupCollectionDynamic[gwv1.Gateway](
		ctx,
		s.istioClient,
		istiogvr.KubernetesGateway_v1,
		krt.WithName("KubeGateways"),
	)

	// alternatively we could start as not synced, and mark ready once ctrl-runtime caches are synced
	proxyTrigger := krt.NewRecomputeTrigger(true)

	glooProxies := krt.NewCollection(kubeGateways, func(kctx krt.HandlerContext, gw *gwv1.Gateway) *glooProxy {
		proxyTrigger.MarkDependant(kctx)
		proxy := s.buildProxy(ctx, gw)
		return proxy
	})
	xdsSnapshots := krt.NewCollection(glooProxies, func(kctx krt.HandlerContext, proxy glooProxy) *xdsSnapWrapper {
		// we are recomputing xds snapshots as proxies have changed, signal that we need to sync xds with these new snapshots
		xdsSnap := s.buildXdsSnapshot(
			ctx,
			kctx,
			logger,
			&proxy,
			configMaps,
			glooEndpoints,
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
	statusReport := krt.NewSingleton(func(kctx krt.HandlerContext) *report {
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

	// latestReport will be constantly updated to contain the merged status report for Kube Gateway status
	// when timer ticks, we will use the state of the mergedReports at that point in time to sync the status to k8s
	var latestReport reports.ReportMap
	latestReport = reports.NewReportMap()
	statusReport.Register(func(o krt.Event[report]) {
		if o.Event == controllers.EventDelete {
			// TODO: handle garbage collection (see: https://github.com/solo-io/solo-projects/issues/7086)
			return
		}
		latestReport = o.Latest().ReportMap
	})

	// kick off the istio informers
	s.istioClient.RunAndWait(ctx.Done())

	// wait for krt collections to sync
	s.istioClient.WaitForCacheSync(
		"kube gw proxy syncer",
		ctx.Done(),
		authConfigs.Synced().HasSynced,
		rlConfigs.Synced().HasSynced,
		configMaps.Synced().HasSynced,
		secrets.Synced().HasSynced,
		services.Synced().HasSynced,
		kubeEndpoints.Synced().HasSynced,
		glooEndpoints.Synced().HasSynced,
		pods.Synced().HasSynced,
		upstreams.Synced().HasSynced,
		glooUpstreams.Synced().HasSynced,
		finalUpstreams.Synced().HasSynced,
		inMemUpstreams.Synced().HasSynced,
		kubeGateways.Synced().HasSynced,
		glooProxies.Synced().HasSynced,
		xdsSnapshots.Synced().HasSynced,
	)

	// wait for ctrl-rtime caches to sync before accepting events
	if !s.mgr.GetCache().WaitForCacheSync(ctx) {
		return errors.New("kube gateway sync loop waiting for all caches to sync failed")
	}

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
				proxyTrigger.TriggerRecomputation()
			}
			go func() {
				s.syncGatewayStatus(ctx, latestReport)
				s.syncRouteStatus(ctx, latestReport)
			}()
			go func() {
				for _, snapWrap := range xdsSnapshots.List() {
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

// ripped from: projects/gloo/pkg/plugins/kubernetes/eds.go#newEndpointsWatcher(...)
func buildEndpoints(
	kctx krt.HandlerContext,
	logger *zap.SugaredLogger,
	FinalUpstreams krt.Collection[upstream],
	KubeEndpoints krt.Collection[*corev1.Endpoints],
	Services krt.Collection[*corev1.Service],
	Pods krt.Collection[*corev1.Pod],
) []*glooEndpoint {
	upstreamSpecs := make(map[*core.ResourceRef]*kubeplugin.UpstreamSpec)
	upstreams := krt.Fetch(kctx, FinalUpstreams)
	for _, us := range upstreams {
		kubeUpstream, ok := us.Upstream.GetUpstreamType().(*gloov1.Upstream_Kube)
		if !ok {
			continue // only care about kube upstreams
		}
		upstreamSpecs[us.GetMetadata().Ref()] = kubeUpstream.Kube
	}
	keps := krt.Fetch(kctx, KubeEndpoints)
	svcs := krt.Fetch(kctx, Services)
	pods := krt.Fetch(kctx, Pods)
	endpoints, warns, errs := kubernetes.FilterEndpoints(
		// there is an unused ctx in the existing function signature so let's just pass an empty ctx.
		// the FilterEndpoints(...) call is being removed in a follow-up so we don't need to mess with it
		context.Background(),
		"gloo-system",
		keps,
		svcs,
		pods,
		upstreamSpecs,
	)
	for _, warn := range warns {
		logger.Warn(warn)
	}
	for _, err := range errs {
		logger.Error(err)
	}
	out := make([]*glooEndpoint, 0, len(endpoints))
	for _, gep := range endpoints {
		out = append(out, &glooEndpoint{gep})
	}
	return out
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
	if proxy != nil {
		translatedGateways = append(translatedGateways, gwplugins.TranslatedGateway{
			Gateway: *gw,
		})
	}

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
	kep krt.Collection[*glooEndpoint],
	ks krt.Collection[*corev1.Secret],
	kus krt.Collection[upstream],
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

	// this must be a solo-kit based kube secret client, we accept a panic if not
	secretResourceClient := s.legacySecretClient.BaseClient().(*kubesecret.ResourceClient)
	gs := make([]*gloov1.Secret, 0, len(secrets))
	for _, i := range secrets {
		secret, err := kubeconverters.GlooSecretConverterChain.FromKubeSecret(ctx, secretResourceClient, i)
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
		gupstreams = append(gupstreams, u.Upstream)
	}
	latestSnap.Upstreams = gupstreams

	geps := endpoints
	eps := UnwrapEps(geps)
	latestSnap.Endpoints = eps

	xdsSnapshot, reports, proxyReport := s.proxyTranslator.buildXdsSnapshot(ctx, proxy.proxy, &latestSnap)
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

// setupCollectionDynamic uses the dynamic client to setup an informer for a resource
// and then uses an intermediate krt collection to type the unstructured resource.
// This is a temporary workaround until we update to the latest istio version and can
// uncomment the code below for registering types.
// HACK: we don't want to use this long term, but it's letting me push forward with deveopment
func setupCollectionDynamic[T any](
	ctx context.Context,
	client kube.Client,
	gvr schema.GroupVersionResource,
	opts ...krt.CollectionOption,
) krt.Collection[*T] {
	delayedClient := kclient.NewDelayedInformer[*unstructured.Unstructured](client, gvr, kubetypes.DynamicInformer, kclient.Filter{})
	mapper := krt.WrapClient(delayedClient, opts...)
	return krt.NewCollection(mapper, func(krtctx krt.HandlerContext, i *unstructured.Unstructured) **T {
		var empty T
		out := &empty
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(i.UnstructuredContent(), out)
		if err != nil {
			contextutils.LoggerFrom(ctx).DPanic("failed converting unstructured into %T: %v", empty, i)
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
			if status := rm.BuildGWStatus(ctx, gw); status != nil {
				if !isGatewayStatusEqual(&gw.Status, status) {
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
// as the Kube GW impl does not support reading Proxies from etcd, the expectation is these proxies are
// written and persisted to the in-memory cache.
// The list MUST contain all valid kube Gw proxies, as the edge reconciler expects the full set; proxies that
// are not added to this list will be garbage collected by the solo-kit base reconciler, so this list must be the
// full SotW.
// The Gloo Xds translator_syncer will receive these proxies via List() using a MultiResourceClient.
// There are two reasons we must make these proxies available to legacy syncer:
// 1. To allow Rate Limit extensions to work, as it only syncs RL configs it finds used on Proxies in the snapshots
// 2. For debug tooling, notably the debug.ProxyEndpointServer
func (s *ProxySyncer) reconcileProxies(ctx context.Context, proxyList gloov1.ProxyList) {
	ctx = contextutils.WithLogger(ctx, "proxyCache")
	logger := contextutils.LoggerFrom(ctx)

	// Proxy CR is located in the writeNamespace, which may be different from the originating Gateway CR
	err := s.proxyReconciler.Reconcile(
		s.writeNamespace,
		proxyList,
		func(original, desired *gloov1.Proxy) (bool, error) {
			// only reconcile if proxies are not equal
			// we reconcile so ggv2 proxies can be used in extension syncing and debug snap storage
			return !proto.Equal(original, desired), nil
		},
		clients.ListOpts{
			Ctx:      ctx,
			Selector: kubeGatewayProxyLabels,
		})
	if err != nil {
		// A write error to our cache should not impact translation
		// We will emit a message, and continue
		logger.Error(err)
	}
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
