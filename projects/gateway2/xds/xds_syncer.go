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
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	gloot "github.com/solo-io/gloo/projects/gateway2/translator"
	"github.com/solo-io/gloo/projects/gateway2/translator/utils"
	xdsutils "github.com/solo-io/gloo/projects/gateway2/xds/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	syncerstats "github.com/solo-io/gloo/projects/gloo/pkg/syncer/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
	sanitizer      sanitizer.XdsSanitizer
	xdsCache       envoycache.SnapshotCache
	controllerName string

	//	latestSnap *v1snap.ApiSnapshot

	xdsGarbageCollection bool

	inputs *XdsInputChannels
	cli    client.Client
	scheme *runtime.Scheme
}

type XdsInputChannels struct {
	genericEvent   AsyncQueue[struct{}]
	discoveryEvent AsyncQueue[DiscoveryInputs]
	secretEvent    AsyncQueue[struct{}]
}

func (x *XdsInputChannels) Kick(ctx context.Context) {
	x.genericEvent.Enqueue(struct{}{})
}

func (x *XdsInputChannels) UpdateDiscoveryInputs(ctx context.Context, inputs DiscoveryInputs) {
	x.discoveryEvent.Enqueue(inputs)
}

func (x *XdsInputChannels) UpdateSecretInputs(ctx context.Context) {
	x.secretEvent.Enqueue(struct{}{})
}

func NewXdsInputChannels() *XdsInputChannels {
	return &XdsInputChannels{
		genericEvent:   NewAsyncQueue[struct{}](),
		discoveryEvent: NewAsyncQueue[DiscoveryInputs](),
		secretEvent:    NewAsyncQueue[struct{}](),
	}
}

func NewXdsSyncer(
	controllerName string,
	sanitizer sanitizer.XdsSanitizer,
	xdsCache envoycache.SnapshotCache,
	xdsGarbageCollection bool,
	inputs *XdsInputChannels,
	cli client.Client,
	scheme *runtime.Scheme,
) *XdsSyncer {
	return &XdsSyncer{
		controllerName:       controllerName,
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
	var discoveryEvent DiscoveryInputs
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
		rm := &reports.ReportMap{
			Gateways: make(map[string]*reports.GatewayReport),
			Routes:   make(map[k8stypes.NamespacedName]*reports.RouteReport),
		}
		r := reports.NewReporter(rm)
		for _, gw := range gwl.Items {
			gw := gw
			lr := t.TranslateProxy(ctx, &gw, queries, r)
			if lr != nil {
				listenersAndRoutesForGateway[&gw] = *lr
			}
			//TODO: handle reports and process statuses
		}
		s.syncEnvoy(ctx, listenersAndRoutesForGateway, discoveryEvent)
		s.syncStatus(ctx, *rm, gwl)
		s.syncRouteStatus(ctx, *rm)
	}

	for {
		select {
		case <-ctx.Done():
			contextutils.LoggerFrom(ctx).Debug("context done, stopping syncer")
			return nil
		case <-s.inputs.genericEvent.Next():
			resyncXds()
		case discoveryEvent = <-s.inputs.discoveryEvent.Next():
			discoveryWarmed = true
			resyncXds()
		case <-s.inputs.secretEvent.Next():
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
func (s *XdsSyncer) syncEnvoy(ctx context.Context, listenersAndRoutesForGateway map[*apiv1.Gateway]gloot.ProxyResult, discoveryEvent DiscoveryInputs) {
	ctx, span := trace.StartSpan(ctx, "gloo.syncer.Sync")
	defer span.End()

	//	s.latestSnap = snap
	logger := log.FromContext(ctx, "pkg", "envoyTranslatorSyncer")
	// snapHash := hashutils.MustHash(snap)
	logger.Info("begin sync",
		"len(gw)", len(listenersAndRoutesForGateway), "len(clusters)", len(discoveryEvent.Clusters), "len(endpoints)", len(discoveryEvent.Endpoints))
	debugLogger := logger.V(1)

	defer logger.Info("end sync")

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if debugLogger.Enabled() {
		//		debugLogger.Info("snap", "snap", syncutil.StringifySnapshot(snap))
	}

	if !s.xdsGarbageCollection {
		var gws []*apiv1.Gateway
		for gw := range listenersAndRoutesForGateway {
			gws = append(gws, gw)
		}
		allKeys := map[string]bool{
			xds.FallbackNodeCacheKey: true,
		}
		// Get all envoy node ID keys
		for _, key := range s.xdsCache.GetStatusKeys() {
			allKeys[key] = false
		}
		// Get all valid node ID keys for Gateways
		for _, key := range xdsutils.SnapshotCacheKeys(gws) {
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

		gwNNs := fmt.Sprintf("%s.%s", gw.Namespace, gw.Name)
		if ctxWithTags, err := tag.New(proxyCtx, tag.Insert(syncerstats.ProxyNameKey, gwNNs)); err == nil {
			proxyCtx = ctxWithTags
		}

		clusters, endpoints := discoveryEvent.Clusters, discoveryEvent.Endpoints

		// replace listeners and routes with the ones we generated
		var listeners []*listenerv3.Listener
		var routes []*routev3.RouteConfiguration
		for _, listenerAndRoutes := range listenersAndRoutes.ListenerAndRoutes {
			routes = append(routes, listenerAndRoutes.RouteConfigs...)
			listeners = append(listeners, listenerAndRoutes.Listener)
		}
		xdsSnapshot := translator.GenerateXDSSnapshot(ctx, utils.EnvoyCacheResourcesListSetToFnvHash, clusters, endpoints, routes, listeners)
		// if validateErr := reports.ValidateStrict(); validateErr != nil {
		// 	logger.Warnw("Proxy had invalid config", zap.Any("proxy", proxy.GetMetadata().Ref()), zap.Error(validateErr))
		// }

		//	sanitizedSnapshot := s.sanitizer.SanitizeSnapshot(ctx, snap, xdsSnapshot, reports)
		// if the snapshot is not consistent, make it so
		xdsSnapshot.MakeConsistent()

		// if validateErr := reports.ValidateStrict(); validateErr != nil {
		// 	logger.Error(validateErr, "Proxy had invalid config after xds sanitization", "proxy", proxy.GetMetadata().Ref())
		// }

		debugLogger.Info("snap", "key", xdsSnapshot)

		// Merge reports after sanitization to capture changes made by the sanitizers
		// reports.Merge(reports)

		key := xdsutils.SnapshotCacheKey(gw)
		s.xdsCache.SetSnapshot(key, xdsSnapshot)

		// Record some metrics
		clustersLen := len(xdsSnapshot.GetResources(types.ClusterTypeV3).Items)
		listenersLen := len(xdsSnapshot.GetResources(types.ListenerTypeV3).Items)
		routesLen := len(xdsSnapshot.GetResources(types.RouteTypeV3).Items)
		endpointsLen := len(xdsSnapshot.GetResources(types.EndpointTypeV3).Items)

		measureResource(proxyCtx, "clusters", clustersLen)
		measureResource(proxyCtx, "listeners", listenersLen)
		measureResource(proxyCtx, "routes", routesLen)
		measureResource(proxyCtx, "endpoints", endpointsLen)

		debugLogger.Info("Setting xDS Snapshot", "key", key,
			"clusters", clustersLen,
			"clustersVersion", xdsSnapshot.GetResources(types.ClusterTypeV3).Version,
			"listeners", listenersLen,
			"listenersVersion", xdsSnapshot.GetResources(types.ListenerTypeV3).Version,
			"routes", routesLen,
			"routesVersion", xdsSnapshot.GetResources(types.RouteTypeV3).Version,
			"endpoints", endpointsLen,
			"endpointsVersion", xdsSnapshot.GetResources(types.EndpointTypeV3).Version)

		debugLogger.Info("Full snapshot for proxy", gwNNs, xdsSnapshot)
	}

	debugLogger.Info("gloo reports to be written")
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
	//	r.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
	//		_, _ = fmt.Fprintf(w, "%+v", prettify(s.latestSnap))
	//	})

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
		gwReport, ok := rm.Gateways[gw.Name]
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
