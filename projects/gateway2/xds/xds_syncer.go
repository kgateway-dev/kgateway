package xds

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/gorilla/mux"
	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	gloot "github.com/solo-io/gloo/projects/gateway2/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	syncerstats "github.com/solo-io/gloo/projects/gloo/pkg/syncer/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// empty resources to give to envoy when a proxy was deleted
const emptyVersionKey = "empty"

var (
	emptyResource = envoycache.Resources{
		Version: emptyVersionKey,
		Items:   map[string]envoycache.Resource{},
	}
	emptySnapshot = xds.NewSnapshotFromResources(
		emptyResource,
		emptyResource,
		emptyResource,
		emptyResource,
	)
)

const (
	// The port used to expose a developer server
	devModePort = 10010
)

var (
	envoySnapshotOut   = stats.Int64("api.gloo.solo.io/translator/resources", "The number of resources in the snapshot in", "1")
	resourceNameKey, _ = tag.NewKey("resource")

	envoySnapshotOutView = &view.View{
		Name:        "api.gloo.solo.io/translator/resources",
		Measure:     envoySnapshotOut,
		Description: "The number of resources in the snapshot for envoy",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{syncerstats.ProxyNameKey, resourceNameKey},
	}
)

func init() {
	_ = view.Register(envoySnapshotOutView)
}

type XdsSyncer struct {
	translator     translator.Translator
	sanitizer      sanitizer.XdsSanitizer
	xdsCache       envoycache.SnapshotCache
	controllerName string

	// used for debugging purposes only
	latestSnap *v1snap.ApiSnapshot

	xdsGarbageCollection bool

	inputs *XdsInputChannels
	cli    client.Client
	scheme *runtime.Scheme
}

type XdsInputChannels struct {
	genericEvent   AsyncQueue[struct{}]
	discoveryEvent AsyncQueue[DiscoveryInputs]
	secretEvent    AsyncQueue[SecretInputs]
}

func (x *XdsInputChannels) Kick(ctx context.Context) {
	x.genericEvent.Enqueue(struct{}{})
}

func (x *XdsInputChannels) UpdateDiscoveryInputs(ctx context.Context, inputs DiscoveryInputs) {
	x.discoveryEvent.Enqueue(inputs)
}

func (x *XdsInputChannels) UpdateSecretInputs(ctx context.Context, inputs SecretInputs) {
	x.secretEvent.Enqueue(inputs)
}

func NewXdsInputChannels() *XdsInputChannels {
	return &XdsInputChannels{
		genericEvent:   NewAsyncQueue[struct{}](),
		discoveryEvent: NewAsyncQueue[DiscoveryInputs](),
		secretEvent:    NewAsyncQueue[SecretInputs](),
	}
}

func NewXdsSyncer(
	controllerName string,
	translator translator.Translator,
	sanitizer sanitizer.XdsSanitizer,
	xdsCache envoycache.SnapshotCache,
	xdsGarbageCollection bool,
	inputs *XdsInputChannels,
	cli client.Client,
	scheme *runtime.Scheme,
) *XdsSyncer {
	return &XdsSyncer{
		controllerName:       controllerName,
		translator:           translator,
		sanitizer:            sanitizer,
		xdsCache:             xdsCache,
		xdsGarbageCollection: xdsGarbageCollection,
		inputs:               inputs,
		cli:                  cli,
		scheme:               scheme,
	}
}

// syncEnvoy will translate, sanatize, and set the snapshot for each of the proxies, all while merging all the reports into allReports.
func (s *XdsSyncer) Start(
	ctx context.Context,
) error {
	proxyApiSnapshot := &v1snap.ApiSnapshot{}
	// proxyApiSnapshot := &v1snap.ApiSnapshot{}
	var (
		discoveryWarmed bool
		secretsWarmed   bool
	)
	resyncXds := func() {
		if !discoveryWarmed || !secretsWarmed {
			return
		}
		var gwl apiv1.GatewayList
		err := s.cli.List(ctx, &gwl)
		if err != nil {
			// This should never happen, try again?
			return
		}
		queries := query.NewData(s.cli, s.scheme)
		t := gloot.NewTranslator()
		listenersAndRoutesForGateway := map[*apiv1.Gateway]gloot.ProxyResult{}
		rm := reports.NewReportMap()
		r := reports.NewReporter(&rm)
		for _, gw := range gwl.Items {
			gw := gw
			lr := t.TranslateProxy(ctx, &gw, queries, r)
			if lr != nil {
				listenersAndRoutesForGateway[&gw] = *lr
			}
			//TODO: handle reports and process statuses
		}
		s.syncEnvoy(ctx, listenersAndRoutesForGateway, proxyApiSnapshot)
		s.syncStatus(ctx, rm, gwl)
		s.syncRouteStatus(ctx, rm)
	}

	for {
		select {
		case <-ctx.Done():
			contextutils.LoggerFrom(ctx).Debug("context done, stopping syncer")
			return nil
		case <-s.inputs.genericEvent.Next():
			resyncXds()
		case discoveryEvent := <-s.inputs.discoveryEvent.Next():
			proxyApiSnapshot.Upstreams = discoveryEvent.Upstreams
			proxyApiSnapshot.Endpoints = discoveryEvent.Endpoints
			discoveryWarmed = true
			resyncXds()
		case secretEvent := <-s.inputs.secretEvent.Next():
			proxyApiSnapshot.Secrets = secretEvent.Secrets
			secretsWarmed = true
			resyncXds()
		}
	}
}

func proxyMetadata(gateway *apiv1.Gateway) *core.Metadata {
	// TODO(ilackarms) what should the proxy ID be
	// ROLE ON ENVOY MUST MATCH <proxy_namespace>~<proxy_name>
	// equal to role: {{.Values.settings.writeNamespace | default .Release.Namespace }}~{{ $name | kebabcase }}
	return &core.Metadata{
		Name:      gateway.Name,
		Namespace: gateway.Namespace,
	}
}

// syncEnvoy will translate, sanatize, and set the snapshot for each of the proxies, all while merging all the reports into allReports.
// NOTE(ilackarms): the below code was copy-pasted (with some deletions) from projects/gloo/pkg/syncer/translator_syncer.go
func (s *XdsSyncer) syncEnvoy(ctx context.Context, listenersAndRoutesForGateway map[*apiv1.Gateway]gloot.ProxyResult, snap *v1snap.ApiSnapshot) reporter.ResourceReports {
	ctx, span := trace.StartSpan(ctx, "gloo.syncer.Sync")
	defer span.End()

	s.latestSnap = snap
	ctx = contextutils.WithLogger(ctx, "envoyTranslatorSyncer")
	logger := contextutils.LoggerFrom(ctx)
	snapHash := hashutils.MustHash(snap)
	logger.Infof("begin sync %v (%v proxies, %v upstreams, %v endpoints, %v secrets, %v artifacts, %v auth configs, %v rate limit configs, %v graphql apis)", snapHash,
		len(snap.Proxies), len(snap.Upstreams), len(snap.Endpoints), len(snap.Secrets), len(snap.Artifacts), len(snap.AuthConfigs), len(snap.Ratelimitconfigs), len(snap.GraphqlApis))
	defer logger.Infof("end sync %v", snapHash)

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		logger.Debug(syncutil.StringifySnapshot(snap))
	}

	reports := make(reporter.ResourceReports)
	reports.Accept(snap.Upstreams.AsInputResources()...)
	reports.Accept(snap.Proxies.AsInputResources()...)

	if !s.xdsGarbageCollection {
		allKeys := map[string]bool{
			xds.FallbackNodeCacheKey: true,
		}
		// Get all envoy node ID keys
		for _, key := range s.xdsCache.GetStatusKeys() {
			allKeys[key] = false
		}
		// Get all valid node ID keys for Proxies
		for _, key := range xds.SnapshotCacheKeys(snap.Proxies) {
			allKeys[key] = true
		}

		// preserve keys from the current list of proxies, set previous invalid snapshots to empty snapshot
		for key, valid := range allKeys {
			if !valid {
				s.xdsCache.SetSnapshot(key, emptySnapshot)
			}
		}
	}
	for gw, listenersAndRoutes := range listenersAndRoutesForGateway {
		proxyCtx := ctx

		proxy := &gloov1.Proxy{
			Metadata: proxyMetadata(gw),
		}

		if ctxWithTags, err := tag.New(proxyCtx, tag.Insert(syncerstats.ProxyNameKey, proxy.GetMetadata().Ref().Key())); err == nil {
			proxyCtx = ctxWithTags
		}

		params := plugins.Params{
			Ctx:      proxyCtx,
			Snapshot: snap,
			Messages: map[*core.ResourceRef][]string{},
		}
		reports := make(reporter.ResourceReports)

		clusters, endpoints := s.translator.TranslateClusterSubsystemComponents(params, proxy, reports)

		// replace listeners and routes with the ones we generated
		var listeners []*listenerv3.Listener
		var routes []*routev3.RouteConfiguration
		for _, listenerAndRoutes := range listenersAndRoutes.ListenerAndRoutes {
			routes = append(routes, listenerAndRoutes.RouteConfigs...)
			listeners = append(listeners, listenerAndRoutes.Listener)
		}
		xdsSnapshot := translator.GenerateXDSSnapshot(ctx, translator.EnvoyCacheResourcesListToFnvHash, clusters, endpoints, routes, listeners)

		// Messages are aggregated during translation, and need to be added to reports
		for _, messages := range params.Messages {
			reports.AddMessages(proxy, messages...)
		}

		// if validateErr := reports.ValidateStrict(); validateErr != nil {
		// 	logger.Warnw("Proxy had invalid config", zap.Any("proxy", proxy.GetMetadata().Ref()), zap.Error(validateErr))
		// }

		sanitizedSnapshot := s.sanitizer.SanitizeSnapshot(ctx, snap, xdsSnapshot, reports)
		// if the snapshot is not consistent, make it so
		xdsSnapshot.MakeConsistent()

		if validateErr := reports.ValidateStrict(); validateErr != nil {
			logger.Warnw("Proxy had invalid config after xds sanitization", zap.Any("proxy", proxy.GetMetadata().Ref()), zap.Error(validateErr))
		}

		// Merge reports after sanitization to capture changes made by the sanitizers
		reports.Merge(reports)
		key := xds.SnapshotCacheKey(proxy)
		s.xdsCache.SetSnapshot(key, sanitizedSnapshot)

		// Record some metrics
		clustersLen := len(xdsSnapshot.GetResources(types.ClusterTypeV3).Items)
		listenersLen := len(xdsSnapshot.GetResources(types.ListenerTypeV3).Items)
		routesLen := len(xdsSnapshot.GetResources(types.RouteTypeV3).Items)
		endpointsLen := len(xdsSnapshot.GetResources(types.EndpointTypeV3).Items)

		measureResource(proxyCtx, "clusters", clustersLen)
		measureResource(proxyCtx, "listeners", listenersLen)
		measureResource(proxyCtx, "routes", routesLen)
		measureResource(proxyCtx, "endpoints", endpointsLen)

		logger.Infow("Setting xDS Snapshot", "key", key,
			"clusters", clustersLen,
			"listeners", listenersLen,
			"routes", routesLen,
			"endpoints", endpointsLen)

		logger.Debugf("Full snapshot for proxy %v: %+v", proxy.GetMetadata().GetName(), xdsSnapshot)
	}

	logger.Debugf("gloo reports to be written: %v", reports)

	return reports
}

// ServeXdsSnapshots exposes Gloo configuration as an API when `devMode` in Settings is True.
// TODO(ilackarms): move this somewhere else, make it part of dev-mode
// https://github.com/solo-io/gloo/issues/6494
func (s *XdsSyncer) ServeXdsSnapshots() error {
	r := mux.NewRouter()

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%v", "Developer API")
	})
	r.HandleFunc("/xds", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%+v", prettify(s.xdsCache.GetStatusKeys()))
	})
	r.HandleFunc("/xds/{key}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		xdsCacheKey := vars["key"]

		xdsSnapshot, _ := s.xdsCache.GetSnapshot(xdsCacheKey)
		_, _ = fmt.Fprintf(w, "%+v", prettify(xdsSnapshot))
	})
	r.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%+v", prettify(s.latestSnap))
	})

	return http.ListenAndServe(fmt.Sprintf(":%d", devModePort), r)
}

func measureResource(ctx context.Context, resource string, len int) {
	if ctxWithTags, err := tag.New(ctx, tag.Insert(resourceNameKey, resource)); err == nil {
		stats.Record(ctxWithTags, envoySnapshotOut.M(int64(len)))
	}
}

func prettify(original interface{}) string {
	b, err := json.MarshalIndent(original, "", "    ")
	if err != nil {
		return ""
	}

	return string(b)
}

func (s *XdsSyncer) syncRouteStatus(ctx context.Context, rm reports.ReportMap) {
	ctx = contextutils.WithLogger(ctx, "routeStatusSyncer")
	logger := contextutils.LoggerFrom(ctx)
	rl := apiv1.HTTPRouteList{}
	err := s.cli.List(ctx, &rl)
	if err != nil {
		logger.Error(err)
		return
	}

	for _, route := range rl.Items {
		routeReport, ok := rm.Routes[client.ObjectKeyFromObject(&route)]
		if !ok {
			//TODO more thought here
			continue
		}
		routeStatus := apiv1.RouteStatus{}
		for _, parentRef := range route.Spec.ParentRefs {
			key := reports.GetParentRefKey(&parentRef)
			parentStatus, ok := routeReport.Parents[key]
			if !ok {
				//todo think
				continue
			}
			if cond := meta.FindStatusCondition(parentStatus.Conditions, string(apiv1.RouteConditionAccepted)); cond == nil {
				parentStatus.SetCondition(reports.HTTPRouteCondition{
					Type:   apiv1.RouteConditionAccepted,
					Status: v1.ConditionTrue,
					Reason: apiv1.RouteReasonAccepted,
				})
			}
			if cond := meta.FindStatusCondition(parentStatus.Conditions, string(apiv1.RouteConditionResolvedRefs)); cond == nil {
				parentStatus.SetCondition(reports.HTTPRouteCondition{
					Type:   apiv1.RouteConditionResolvedRefs,
					Status: v1.ConditionTrue,
					Reason: apiv1.RouteReasonResolvedRefs,
				})
			}

			//TODO add logic for partially invalid condition

			finalConditions := make([]v1.Condition, 0)
			for _, pCondition := range parentStatus.Conditions {
				pCondition.ObservedGeneration = route.Generation       // don't have generation is the report, should consider adding it
				pCondition.LastTransitionTime = v1.NewTime(time.Now()) // same as above, should calculate at report time possibly
				finalConditions = append(finalConditions, pCondition)
			}

			routeParentStatus := apiv1.RouteParentStatus{
				ParentRef:      parentRef,
				ControllerName: apiv1.GatewayController(s.controllerName),
				Conditions:     finalConditions,
			}
			routeStatus.Parents = append(routeStatus.Parents, routeParentStatus)
		}
		route.Status = apiv1.HTTPRouteStatus{
			RouteStatus: routeStatus,
		}
		if err := s.cli.Status().Update(ctx, &route); err != nil {
			logger.Error(err)
		}
	}
}

func (s *XdsSyncer) syncStatus(ctx context.Context, rm reports.ReportMap, gwl apiv1.GatewayList) {
	ctx = contextutils.WithLogger(ctx, "statusSyncer")
	logger := contextutils.LoggerFrom(ctx)
	//TODO(Law): bail out early if possible
	//TODO(Law): do another Get on the gw, possibly use Patch for status subresource modification
	//TODO(Law): add generation changed predicate
	for _, gw := range gwl.Items {
		gwReport, ok := rm.Gateways[client.ObjectKeyFromObject(&gw)]
		if !ok {
			// why don't we have a report for this gateway?
			continue
		}
		//TODO(Law): deterministic sorting
		finalListeners := make([]apiv1.ListenerStatus, 0)
		for _, lis := range gw.Spec.Listeners {
			lisReport, ok := gwReport.Listeners[string(lis.Name)]
			if !ok {
				//TODO(Law): why don't we have a report for a listner? shouldnt happen
				continue
			}

			// set healthy conditions for Condition Types not set yet (i.e. no negative status yet, we can assume positive)
			if cond := meta.FindStatusCondition(lisReport.Status.Conditions, string(apiv1.ListenerConditionAccepted)); cond == nil {
				lisReport.SetCondition(reports.ListenerCondition{
					Type:   apiv1.ListenerConditionAccepted,
					Status: v1.ConditionTrue,
					Reason: apiv1.ListenerReasonAccepted,
				})
			}
			if cond := meta.FindStatusCondition(lisReport.Status.Conditions, string(apiv1.ListenerConditionConflicted)); cond == nil {
				lisReport.SetCondition(reports.ListenerCondition{
					Type:   apiv1.ListenerConditionConflicted,
					Status: v1.ConditionFalse,
					Reason: apiv1.ListenerReasonNoConflicts,
				})
			}
			if cond := meta.FindStatusCondition(lisReport.Status.Conditions, string(apiv1.ListenerConditionResolvedRefs)); cond == nil {
				lisReport.SetCondition(reports.ListenerCondition{
					Type:   apiv1.ListenerConditionResolvedRefs,
					Status: v1.ConditionTrue,
					Reason: apiv1.ListenerReasonResolvedRefs,
				})
			}
			if cond := meta.FindStatusCondition(lisReport.Status.Conditions, string(apiv1.ListenerConditionProgrammed)); cond == nil {
				lisReport.SetCondition(reports.ListenerCondition{
					Type:   apiv1.ListenerConditionProgrammed,
					Status: v1.ConditionTrue,
					Reason: apiv1.ListenerReasonProgrammed,
				})
			}

			finalConditions := make([]v1.Condition, 0)
			oldLisStatusIndex := slices.IndexFunc(gw.Status.Listeners, func(l apiv1.ListenerStatus) bool {
				return l.Name == lis.Name
			})
			for _, lisCondition := range lisReport.Status.Conditions {
				lisCondition.ObservedGeneration = gw.Generation // don't have generation is the report, should consider adding it
				// copy the old condition from the gw so last transition time is set correctly
				if oldLisStatusIndex != -1 {
					if cond := meta.FindStatusCondition(gw.Status.Listeners[oldLisStatusIndex].Conditions, lisCondition.Type); cond != nil {
						finalConditions = append(finalConditions, *cond)
					}
				}

				meta.SetStatusCondition(&finalConditions, lisCondition)
			}
			lisReport.Status.Conditions = finalConditions
			finalListeners = append(finalListeners, lisReport.Status)
		}

		// set missing conditions, i.e. set healthy conditions
		if cond := meta.FindStatusCondition(gwReport.Conditions, string(apiv1.GatewayConditionAccepted)); cond == nil {
			gwReport.SetCondition(reports.GatewayCondition{
				Type:   apiv1.GatewayConditionAccepted,
				Status: v1.ConditionTrue,
				Reason: apiv1.GatewayReasonAccepted,
			})
		}
		if cond := meta.FindStatusCondition(gwReport.Conditions, string(apiv1.GatewayConditionProgrammed)); cond == nil {
			gwReport.SetCondition(reports.GatewayCondition{
				Type:   apiv1.GatewayConditionProgrammed,
				Status: v1.ConditionTrue,
				Reason: apiv1.GatewayReasonProgrammed,
			})
		}

		// recalculate top-level GatewayStatus
		finalGwStatus := apiv1.GatewayStatus{}
		finalConditions := make([]v1.Condition, 0)
		for _, gwCondition := range gwReport.Conditions {
			gwCondition.ObservedGeneration = gw.Generation
			// copy the old condition from the gw so last transition time is set correctly
			if cond := meta.FindStatusCondition(gw.Status.Conditions, gwCondition.Type); cond != nil {
				finalConditions = append(finalConditions, *cond)
			}
			meta.SetStatusCondition(&finalConditions, gwCondition)
		}

		finalGwStatus.Conditions = finalConditions
		finalGwStatus.Listeners = finalListeners
		gw.Status = finalGwStatus
		if err := s.cli.Status().Patch(ctx, &gw, client.Merge); err != nil {
			logger.Error(err)
		}
	}
}
