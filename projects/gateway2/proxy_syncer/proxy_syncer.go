package proxy_syncer

import (
	"context"
	"strconv"

	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	gwv2_translator "github.com/solo-io/gloo/projects/gateway2/translator"
	gwplugins "github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	gloo_solo_io "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// QueueStatusForProxiesFn queues a list of proxies to be synced and the plugin registry that produced them for a given sync iteration
type QueueStatusForProxiesFn func(proxies gloo_solo_io.ProxyList, pluginRegistry *registry.PluginRegistry, totalSyncCount int)

// ProxySyncer is responsible for translating Kubernetes Gateway CRs into Gloo Proxies
// and syncing the proxyClient with the newly translated proxies.
type ProxySyncer struct {
	controllerName string
	writeNamespace string

	inputs          *GatewayInputChannels
	mgr             manager.Manager
	k8sGwExtensions extensions.K8sGatewayExtensions

	// proxyReconciler wraps the client that writes Proxy resources into an in-memory cache
	// This cache is utilized by the debug.ProxyEndpointServer
	proxyReconciler gloo_solo_io.ProxyReconciler

	// queueStatusForProxies stores a list of proxies that need the proxy status synced and the plugin registry
	// that produced them for a given sync iteration
	queueStatusForProxies QueueStatusForProxiesFn
}

type GatewayInputChannels struct {
	genericEvent AsyncQueue[struct{}]
	secretEvent  AsyncQueue[SecretInputs]
}

func (x *GatewayInputChannels) Kick(ctx context.Context) {
	x.genericEvent.Enqueue(struct{}{})
}

func (x *GatewayInputChannels) UpdateSecretInputs(ctx context.Context, inputs SecretInputs) {
	x.secretEvent.Enqueue(inputs)
}

func NewGatewayInputChannels() *GatewayInputChannels {
	return &GatewayInputChannels{
		genericEvent: NewAsyncQueue[struct{}](),
		secretEvent:  NewAsyncQueue[SecretInputs](),
	}
}

var (
	// labels used to uniquely identify Proxies that are managed by the kube gateway controller
	kubeGatewayProxyLabels = map[string]string{
		utils.ProxyTypeKey: utils.GatewayApiProxyValue,
	}
)

// NewProxySyncer returns an implementation of the ProxySyncer
// The provided GatewayInputChannels are used to trigger syncs.
// The proxy sync is triggered by the `genericEvent` which is kicked when
// we reconcile gateway in the gateway controller. The `secretEvent` is kicked when a secret is created, updated,
func NewProxySyncer(
	controllerName, writeNamespace string,
	inputs *GatewayInputChannels,
	mgr manager.Manager,
	k8sGwExtensions extensions.K8sGatewayExtensions,
	proxyClient gloo_solo_io.ProxyClient,
	queueStatusForProxies QueueStatusForProxiesFn,
) *ProxySyncer {
	return &ProxySyncer{
		controllerName:        controllerName,
		writeNamespace:        writeNamespace,
		inputs:                inputs,
		mgr:                   mgr,
		k8sGwExtensions:       k8sGwExtensions,
		proxyReconciler:       gloo_solo_io.NewProxyReconciler(proxyClient, statusutils.NewNoOpStatusClient()),
		queueStatusForProxies: queueStatusForProxies,
	}
}

func (s *ProxySyncer) Start(ctx context.Context) error {
	ctx = contextutils.WithLogger(ctx, "k8s-gw-syncer")
	contextutils.LoggerFrom(ctx).Debug("starting syncer for k8s gateway proxies")

	var (
		secretsWarmed bool
		// totalResyncs is used to track the number of times the proxy syncer has been triggered
		totalResyncs int
	)
	resyncProxies := func() {
		if !secretsWarmed {
			return
		}
		totalResyncs++
		contextutils.LoggerFrom(ctx).Debugf("resyncing k8s gateway proxies [%v]", totalResyncs)

		var gwl apiv1.GatewayList
		err := s.mgr.GetClient().List(ctx, &gwl)
		if err != nil {
			// This should never happen, try again?
			return
		}

		gatewayQueries := query.NewData(s.mgr.GetClient(), s.mgr.GetScheme())

		pluginRegistry := s.k8sGwExtensions.CreatePluginRegistry(ctx)
		gatewayTranslator := gwv2_translator.NewTranslator(gatewayQueries, pluginRegistry)

		rm := reports.NewReportMap()
		r := reports.NewReporter(&rm)

		var (
			proxies            gloo_solo_io.ProxyList
			translatedGateways []gwplugins.TranslatedGateway
		)
		for _, gw := range gwl.Items {
			proxy := gatewayTranslator.TranslateProxy(ctx, &gw, s.writeNamespace, r)
			if proxy != nil {
				// Add proxy id to the proxy metadata to track proxies for status reporting
				proxyAnnotations := proxy.GetMetadata().GetAnnotations()
				if proxyAnnotations == nil {
					proxyAnnotations = make(map[string]string)
				}
				proxyAnnotations[utils.ProxySyncId] = strconv.Itoa(totalResyncs)
				proxy.GetMetadata().Annotations = proxyAnnotations

				proxies = append(proxies, proxy)
				translatedGateways = append(translatedGateways, gwplugins.TranslatedGateway{
					Gateway: gw,
				})
			}
		}

		applyPostTranslationPlugins(ctx, pluginRegistry, &gwplugins.PostTranslationContext{
			TranslatedGateways: translatedGateways,
		})

		s.queueStatusForProxies(proxies, &pluginRegistry, totalResyncs)
		s.syncStatus(ctx, rm, gwl)
		s.syncRouteStatus(ctx, rm)
		s.reconcileProxies(ctx, proxies)
	}

	for {
		select {
		case <-ctx.Done():
			contextutils.LoggerFrom(ctx).Debug("context done, stopping proxy syncer")
			return nil
		case <-s.inputs.genericEvent.Next():
			resyncProxies()
		case <-s.inputs.secretEvent.Next():
			secretsWarmed = true
			resyncProxies()
		}
	}
}

func (s *ProxySyncer) syncRouteStatus(ctx context.Context, rm reports.ReportMap) {
	ctx = contextutils.WithLogger(ctx, "routeStatusSyncer")
	logger := contextutils.LoggerFrom(ctx)
	rl := apiv1.HTTPRouteList{}
	err := s.mgr.GetClient().List(ctx, &rl)
	if err != nil {
		logger.Error(err)
		return
	}

	for _, route := range rl.Items {
		route := route // pike
		if status := rm.BuildRouteStatus(ctx, route, s.controllerName); status != nil {
			route.Status = *status
			if err := s.mgr.GetClient().Status().Update(ctx, &route); err != nil {
				logger.Error(err)
			}
		}
	}
}

// syncStatus updates the status of the Gateway CRs
func (s *ProxySyncer) syncStatus(ctx context.Context, rm reports.ReportMap, gwl apiv1.GatewayList) {
	ctx = contextutils.WithLogger(ctx, "statusSyncer")
	logger := contextutils.LoggerFrom(ctx)
	for _, gw := range gwl.Items {
		gw := gw // pike
		if status := rm.BuildGWStatus(ctx, gw); status != nil {
			gw.Status = *status
			if err := s.mgr.GetClient().Status().Patch(ctx, &gw, client.Merge); err != nil {
				logger.Error(err)
			}
		}
	}
}

// reconcileProxies persists the proxies that were generated during translations and stores them in an in-memory cache
// This cache is utilized by the debug.ProxyEndpointServer
// As well as to resync the Gloo Xds Translator (when it receives new proxies using a MultiResourceClient)
func (s *ProxySyncer) reconcileProxies(ctx context.Context, proxyList gloo_solo_io.ProxyList) {
	ctx = contextutils.WithLogger(ctx, "proxyCache")
	logger := contextutils.LoggerFrom(ctx)

	// Proxy CR is located in the writeNamespace, which may be different from the originating Gateway CR
	err := s.proxyReconciler.Reconcile(
		s.writeNamespace,
		proxyList,
		func(original, desired *gloo_solo_io.Proxy) (bool, error) {
			// always update
			return true, nil
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
