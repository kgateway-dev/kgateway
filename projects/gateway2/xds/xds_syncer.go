package xds

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	gloot "github.com/solo-io/gloo/projects/gateway2/translator"
	gloo_solo_io "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	syncerstats "github.com/solo-io/gloo/projects/gloo/pkg/syncer/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
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
	emptyResource = cache.Resources{
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
	translator translator.Translator
	sanitizer  sanitizer.XdsSanitizer
	xdsCache   envoycache.SnapshotCache

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
	translator translator.Translator,
	sanitizer sanitizer.XdsSanitizer,
	xdsCache envoycache.SnapshotCache,
	xdsGarbageCollection bool,
	inputs *XdsInputChannels,
	cli client.Client,
	scheme *runtime.Scheme,
) *XdsSyncer {
	return &XdsSyncer{
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
		proxies := gloo_solo_io.ProxyList{}
		rm := &reports.ReportMap{Gateways: make(map[string]*reports.GatewayReport)}
		r := reports.NewReporter(rm)
		for _, gw := range gwl.Items {
			proxy := t.TranslateProxy(ctx, &gw, queries, r)
			if proxy != nil {
				proxies = append(proxies, proxy)
				//TODO: handle reports and process statuses
			}
		}
		proxyApiSnapshot.Proxies = proxies
		s.syncEnvoy(ctx, proxyApiSnapshot)
		s.syncStatus(ctx, *rm, gwl)
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

// syncEnvoy will translate, sanatize, and set the snapshot for each of the proxies, all while merging all the reports into allReports.
// NOTE(ilackarms): the below code was copy-pasted (with some deletions) from projects/gloo/pkg/syncer/translator_syncer.go
func (s *XdsSyncer) syncEnvoy(ctx context.Context, snap *v1snap.ApiSnapshot) reporter.ResourceReports {
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
	for _, proxy := range snap.Proxies {
		proxyCtx := ctx
		if ctxWithTags, err := tag.New(proxyCtx, tag.Insert(syncerstats.ProxyNameKey, proxy.GetMetadata().Ref().Key())); err == nil {
			proxyCtx = ctxWithTags
		}

		params := plugins.Params{
			Ctx:      proxyCtx,
			Snapshot: snap,
			Messages: map[*core.ResourceRef][]string{},
		}

		xdsSnapshot, reports, _ := s.translator.Translate(params, proxy)

		// Messages are aggregated during translation, and need to be added to reports
		for _, messages := range params.Messages {
			reports.AddMessages(proxy, messages...)
		}

		if validateErr := reports.ValidateStrict(); validateErr != nil {
			logger.Warnw("Proxy had invalid config", zap.Any("proxy", proxy.GetMetadata().Ref()), zap.Error(validateErr))
		}

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

func (s *XdsSyncer) syncStatus(ctx context.Context, rm reports.ReportMap, gwl apiv1.GatewayList) {
	ctx = contextutils.WithLogger(ctx, "envoyTranslatorSyncer")
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

			finalConditions := make([]v1.Condition, 0)
			for _, lisCondition := range lisReport.Status.Conditions {
				lisCondition.ObservedGeneration = gw.Generation          // don't have generation is the report, should consider adding it
				lisCondition.LastTransitionTime = v1.NewTime(time.Now()) // same as above, should calculate at report time possibly
				finalConditions = append(finalConditions, lisCondition)
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
			gwCondition.ObservedGeneration = gw.Generation          // don't have generation is the report, should consider adding it
			gwCondition.LastTransitionTime = v1.NewTime(time.Now()) // same as above, should calculate at report time possibly
			finalConditions = append(finalConditions, gwCondition)
		}
		finalGwStatus.Conditions = finalConditions
		finalGwStatus.Listeners = finalListeners
		gw.Status = finalGwStatus
		if err := s.cli.Status().Update(ctx, &gw); err != nil {
			logger.Error(err)
		}
	}
}
