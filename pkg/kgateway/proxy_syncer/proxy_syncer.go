package proxy_syncer

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	kmetrics "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	plug "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

var _ manager.LeaderElectionRunnable = &ProxySyncer{}

// ProxySyncer orchestrates the translation of K8s Gateway CRs to xDS
// and setting the output xDS snapshot in the envoy snapshot cache,
// resulting in each connected proxy getting the correct configuration.
// It runs on all pods (leader or follower) as the xDS snapshot must be consistent across pods.
// It also derives per-object desired-status collections from the translation reports;
// the StatusSyncer attaches them to a write queue on the leader.
type ProxySyncer struct {
	controllerName string

	commonCols *collections.CommonCollections
	translator *translator.CombinedTranslator
	plugins    plug.Plugin

	apiClient       apiclient.Client
	proxyTranslator ProxyTranslator
	// scoping, including the plugins' emission claims, is resolved once at
	// construction.
	scoping clusterScoping

	uniqueClients krt.Collection[ir.UniquelyConnectedClient]
	// clientState is the synchronous state behind uniqueClients; the reclaimer
	// checks departure through it.
	clientState krtcollections.XDSClientState

	statusContributions         krt.Collection[reports.StatusContribution]
	statusContributionsByTarget krt.Index[reports.StatusKey, reports.StatusContribution]
	mostXdsSnapshots            krt.Collection[GatewayXdsResources]
	perclientSnapCollection     krt.Collection[XdsSnapWrapper]

	statusCollections *statussync.StatusCollections
	statusWriters     map[schema.GroupVersionKind]statussync.ResourceStatusSyncer

	// extraPolicyTargetResolvers are the resolvers registered with WithPolicyTargetResolver.
	extraPolicyTargetResolvers policyTargetResolvers

	waitForSync []cache.InformerSynced
	ready       atomic.Bool
}

type GatewayXdsResources struct {
	types.NamespacedName
	// Clusters are items in the CDS response payload.
	// +krtEqualsTodo include CDC resources in equality for diff detection
	Clusters     []envoycachetypes.ResourceWithTTL
	ClustersHash uint64

	// Routes are items in the RDS response payload.
	Routes envoycache.Resources

	// Listeners are items in the LDS response payload.
	Listeners envoycache.Resources

	// ReferencedClusters are the routing targets of Routes and Listeners.
	// +noKrtEquals derived from Routes and Listeners, whose content-hash versions Equals compares
	ReferencedClusters map[string]struct{}

	// EmittedClusters over-approximates the cluster names Routes and Listeners could
	// name, plus selectors that make filtering unsafe. ReferencedClusters is always
	// a subset of its Names.
	EmittedClusters emittedClusters

	// Secrets are items in the SDS response payload.
	Secrets envoycache.Resources
}

func (r GatewayXdsResources) ResourceName() string {
	return xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, r.Namespace, r.Name)
}

func (r GatewayXdsResources) Equals(in GatewayXdsResources) bool {
	return r.NamespacedName == in.NamespacedName &&
		r.ClustersHash == in.ClustersHash &&
		r.Routes.Version == in.Routes.Version &&
		r.Listeners.Version == in.Listeners.Version &&
		r.Secrets.Version == in.Secrets.Version &&
		r.EmittedClusters.Equals(in.EmittedClusters)
}

// GatewayStatusSnapshot is the status-only half of one Gateway translation. Keeping it a
// separate field of gatewayTranslationOutput, with its own Equals, is what stops a
// status-only change from invalidating the xDS projection derived from the same output.
//
// It is not itself a KRT collection element -- gatewayStatusContributions unpacks it straight
// out of the translation output -- so it needs no ResourceName.
type GatewayStatusSnapshot struct {
	types.NamespacedName
	Contributions []reports.StatusContribution
}

func (r GatewayStatusSnapshot) Equals(other GatewayStatusSnapshot) bool {
	return r.NamespacedName == other.NamespacedName &&
		slices.EqualFunc(r.Contributions, other.Contributions, func(a, b reports.StatusContribution) bool {
			return a.Equals(b)
		})
}

type gatewayTranslationOutput struct {
	Xds    GatewayXdsResources
	Status GatewayStatusSnapshot
}

func (r gatewayTranslationOutput) ResourceName() string {
	return r.Xds.ResourceName()
}

func (r gatewayTranslationOutput) Equals(other gatewayTranslationOutput) bool {
	return r.Xds.Equals(other.Xds) && r.Status.Equals(other.Status)
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
	return envoycache.NewResourcesWithTTL(strconv.FormatUint(h, 10), r)
}

// toTranslationOutput projects one gateway translation into published xDS and
// status. scoping gates the emitted-set walk (emittedClustersFor).
func toTranslationOutput(gw ir.Gateway, xdsSnap irtranslator.TranslationResult, r reports.ReportMap, scoping clusterScoping) *gatewayTranslationOutput {
	c, ch := sliceToResourcesHash(xdsSnap.ExtraClusters)
	routes := sliceToResources(xdsSnap.Routes)
	listeners := sliceToResources(xdsSnap.Listeners)
	nn := types.NamespacedName{
		Namespace: gw.Obj.GetNamespace(),
		Name:      gw.Obj.GetName(),
	}
	return &gatewayTranslationOutput{
		Xds: GatewayXdsResources{
			NamespacedName:     nn,
			ClustersHash:       ch,
			Clusters:           c,
			Routes:             routes,
			Listeners:          listeners,
			Secrets:            sliceToResources(xdsSnap.Secrets),
			ReferencedClusters: collectReferencedClusters(routes, listeners),
			EmittedClusters:    emittedClustersFor(scoping, routes, listeners),
		},
		Status: GatewayStatusSnapshot{
			NamespacedName: nn,
			Contributions: reports.StatusContributionsFromReportMap(reports.StatusSource{
				Kind: reports.GatewayStatusSource,
				Name: nn.String(),
			}, r),
		},
	}
}

// NewProxySyncer returns a ProxySyncer runnable
// The provided GatewayInputChannels are used to trigger syncs.
func NewProxySyncer(
	ctx context.Context,
	controllerName string,
	client apiclient.Client,
	uniqueClients krt.Collection[ir.UniquelyConnectedClient],
	mergedPlugins plug.Plugin,
	commonCols *collections.CommonCollections,
	xdsCache envoycache.SnapshotCache,
	validator validator.Validator,
	xdsClientState krtcollections.XDSClientState,
	opts ...StatusSyncerOption,
) *ProxySyncer {
	optCfg := processStatusSyncerOptions(opts...)
	if xdsClientState == nil {
		// Nothing fails without it, so integrations that build their own
		// StartConfig would lose this protection silently.
		logger.Warn("no xDS client state wired; every reconnecting proxy is treated as a new one, so a deferred first publish to a proxy still serving config is not withheld past the first-publish budget, a returning proxy is answered from its previous connection's snapshot until the next publish to it, and departed proxies' snapshots are never reclaimed")
	}
	scoping := clusterScopingFrom(commonCols.Settings, mergedPlugins.ContributesPolicies)
	return &ProxySyncer{
		controllerName: controllerName,
		commonCols:     commonCols,
		apiClient:      client,
		proxyTranslator: NewProxyTranslator(xdsCache, xdsClientState, commonCols.Settings.PerClientPublishBudget, commonCols.Settings.XdsSnapshotConsistencyCheck,
			scoping),
		scoping:                    scoping,
		uniqueClients:              uniqueClients,
		clientState:                xdsClientState,
		translator:                 translator.NewCombinedTranslator(ctx, mergedPlugins, commonCols, validator),
		plugins:                    mergedPlugins,
		extraPolicyTargetResolvers: optCfg.policyTargetResolvers,
	}
}

type ProxyTranslator struct {
	xdsCache envoycache.SnapshotCache
	// gate bounds first-publish and held-flip waits for unready clusters
	// (publish_gate.go). A pointer, so ProxyTranslator copies share its state.
	gate *publishGate
}

// NewProxyTranslator builds the publishing half of the syncer. A nil
// xdsClientState treats every client as cold; scoping sets the de-reference
// grace.
func NewProxyTranslator(
	xdsCache envoycache.SnapshotCache,
	xdsClientState krtcollections.XDSClientState,
	publishBudget time.Duration,
	checkSnapshotConsistency bool,
	scoping clusterScoping,
) ProxyTranslator {
	return ProxyTranslator{
		xdsCache: xdsCache,
		gate:     newPublishGate(publishBudget, checkSnapshotConsistency, xdsClientState, scoping),
	}
}

// registerConnectionStartHooks clears a key's cache entry and gate state when
// its first stream connects, if state implements
// krtcollections.ConnectionStartHooks, and reports whether it does.
//
// go-control-plane answers a returning client's first requests from its last
// cache entry, which can predate config from another controller replica.
// Serving it rolls the proxy back until the first publish, which a deferred
// build can withhold for the first-publish budget or warm withhold bound.
//
// During admission, before watches exist and with no other stream of the key
// connected, the fence advances the generation and clears the entry. It also
// rejects publishes computed for the previous connection (connectionFence).
// After admission, the gate resets, adopts the generation and publishes the
// current row; a quick reconnect may produce no new row, leaving the stream
// waiting for the next config change.
//
// A stream joining a connected key keeps the existing entry. currentRow
// returns the client's current row, or nil.
func (t ProxyTranslator) registerConnectionStartHooks(ctx context.Context, state krtcollections.XDSClientState, currentRow func(proxyKey string) *XdsSnapWrapper) bool {
	hooks, ok := state.(krtcollections.ConnectionStartHooks)
	if !ok {
		return false
	}
	hooks.SetConnectionStartHooks(
		func(proxyKey string) {
			t.gate.fence.start(t.xdsCache, proxyKey)
		},
		func(proxyKey string) {
			t.gate.connectionStarted(proxyKey)
			if row := currentRow(proxyKey); row != nil {
				t.syncXds(ctx, *row)
			}
		},
	)
	return true
}

// clientSnapshotDeleted withdraws a connected client's snapshot row
// (publishGate.snapshotWithdrawn) or ends a departed client's connection.
func (t ProxyTranslator) clientSnapshotDeleted(proxyKey string, connected bool) {
	if connected {
		t.gate.snapshotWithdrawn(proxyKey)
		return
	}
	t.gate.clientDeparted(proxyKey)
}

var logger = logging.New("proxy_syncer")

func (s *ProxySyncer) Init(ctx context.Context, krtopts krtutil.KrtOptions) {
	queries := query.NewData(s.commonCols)

	gatewayBackendVariants := newGatewayBackendVariants(
		ctx,
		krtopts,
		queries,
		s.commonCols.GatewayIndex.Gateways,
	)
	gatewayBackendVariantBackends := krt.NewCollection(gatewayBackendVariants, func(kctx krt.HandlerContext, backendForGateway gatewayScopedBackend) *ir.BackendObjectIR {
		if backendForGateway.backend == nil {
			return nil
		}
		backend := *backendForGateway.backend
		return &backend
	}, krtopts.ToOptions("GatewayBackendClientCertificateVariantBackends")...)
	gatewayBackendVariantBackendsWithPolicy, _ := s.commonCols.BackendIndex.AttachPoliciesToCollection(
		gatewayBackendVariantBackends,
		"GatewayBackendClientCertificateVariantBackendsWithPolicy",
	)
	gatewayBackendVariantEndpoints := newGatewayBackendVariantEndpoints(krtopts, gatewayBackendVariants, s.commonCols.Endpoints)

	// all backends with policies attached in a single collection
	finalBackends := krt.JoinCollection(
		append(s.commonCols.BackendIndex.BackendsWithPolicy(), gatewayBackendVariantBackendsWithPolicy),
		// WithJoinUnchecked enables a more optimized lookup on the hotpath by assuming we do not have any overlapping ResourceName
		// in the backend collection.
		append(krtopts.ToOptions("FinalBackends"), krt.WithJoinUnchecked())...)
	finalBackendsWithPolicyStatus := krt.JoinCollection(s.commonCols.BackendIndex.BackendsWithPolicyRequiringStatus(),
		// WithJoinUnchecked enables a more optimized lookup on the hotpath by assuming we do not have any overlapping ResourceName
		// in the backend collection.
		append(krtopts.ToOptions("FinalBackendsWithPolicyStatus"), krt.WithJoinUnchecked())...)
	allEndpoints := krt.JoinCollection(
		[]krt.Collection[ir.EndpointsForBackend]{s.commonCols.Endpoints, gatewayBackendVariantEndpoints},
		krtopts.ToOptions("AllEndpoints")...,
	)

	s.translator.Init(ctx)

	// Resolved at construction; every scoping path checks ScopesClusters() and
	// does nothing when it is false.
	scoping := s.scoping
	translationOutputs := krt.NewCollection(s.commonCols.GatewayIndex.Gateways, func(kctx krt.HandlerContext, gw ir.Gateway) *gatewayTranslationOutput {
		// Note: s.commonCols.GatewayIndex.Gateways is already filtered to only include Gateways
		// with controllerName matching s.controllerName (envoy controller). The filtering happens
		// in GatewaysForEnvoyTransformationFunc in pkg/krtcollections/policy.go
		logger.Debug("building proxy for kube gw", "name", client.ObjectKeyFromObject(gw.Obj), "version", gw.Obj.GetResourceVersion())

		xdsSnap, rm := s.translator.TranslateGateway(kctx, ctx, gw)
		if xdsSnap == nil {
			return nil
		}

		return toTranslationOutput(gw, *xdsSnap, rm, scoping)
	}, krtopts.ToOptions("GatewayTranslationOutputs")...)
	s.commonCols.GatewayIndex.Gateways.Register(func(e krt.Event[ir.Gateway]) {
		if e.Event == controllers.EventDelete {
			s.translator.ForgetGateway(types.NamespacedName{Namespace: e.Old.Namespace, Name: e.Old.Name})
		}
	})
	s.mostXdsSnapshots = krt.NewCollection(translationOutputs, func(_ krt.HandlerContext, output gatewayTranslationOutput) *GatewayXdsResources {
		return &output.Xds
	}, krtopts.ToOptions("MostXdsSnapshots")...)
	epPerClient := NewPerClientEnvoyEndpoints(
		krtopts,
		s.uniqueClients,
		newFinalBackendEndpoints(krtopts, finalBackends, allEndpoints),
		s.translator.ResolveEndpoints,
		s.translator.BuildClusterLoadAssignment,
	)
	localClusterEpPerClient := NewPerClientLocalClusterEndpoints(
		krtopts,
		s.uniqueClients,
		s.commonCols.LocalityPods,
	)
	clustersPerClient := NewPerClientEnvoyClusters(
		ctx,
		krtopts,
		s.translator.GetBackendTranslator(),
		finalBackends,
		s.uniqueClients,
	)

	s.perclientSnapCollection = snapshotPerClient(
		krtopts,
		s.uniqueClients,
		s.mostXdsSnapshots,
		epPerClient,
		clustersPerClient,
		scoping,
		localClusterEpPerClient,
	)

	backendPolicyContributions := backendPolicyStatusContributions(finalBackendsWithPolicyStatus, clustersPerClient.StatusClusters(), krtopts)

	// Backend status is reduced per Backend. Indexed cluster and plugin-condition
	// dependencies ensure one client's error only recomputes its owning Backend.
	kgwBackendPlugin := s.plugins.ContributesBackends[wellknown.BackendGVK.GroupKind()]
	kgwBackendCol := kgwBackendPlugin.Backends
	kgwBackendExtraConditions := kgwBackendPlugin.ExtraConditions
	if kgwBackendCol == nil {
		kgwBackendCol = krt.NewStaticCollection[ir.BackendObjectIR](nil, nil, krtopts.ToOptions("NoBackendStatusInputs")...)
	}
	if kgwBackendExtraConditions == nil {
		kgwBackendExtraConditions = krt.NewStaticCollection[ir.BackendObjectStatus](nil, nil, krtopts.ToOptions("NoBackendExtraConditions")...)
	}
	backendContributions := backendStatusContributions(
		kgwBackendCol,
		clustersPerClient.StatusClusters(),
		kgwBackendExtraConditions,
		krtopts,
	)

	// Gateway and Backend translation only ever report on policies that attached to
	// something. A policy whose targetRef names a missing object attaches nowhere, so this
	// producer checks every policy's own targetRefs and reports the unresolved ones. See
	// policy_target_status.go.
	var policyCols []krt.Collection[ir.PolicyWrapper]
	for _, plugin := range s.plugins.ContributesPolicies {
		if plugin.Policies != nil {
			policyCols = append(policyCols, plugin.Policies)
		}
	}
	allPolicies := krt.JoinCollection(policyCols, krtopts.ToOptions("PolicyTargetPolicies")...)
	policyTargetContributions := policyTargetStatusContributions(allPolicies,
		newPolicyTargetResolvers(s.commonCols, s.plugins.ContributesBackends, s.extraPolicyTargetResolvers), krtopts)

	// All status paths now meet as independently keyed contributions. Policy
	// ancestors from Gateway translation, Backend translation, and targetRef
	// resolution naturally reduce under the same policy key without a competing
	// singleton writer.
	s.statusContributions = krt.JoinCollection([]krt.Collection[reports.StatusContribution]{
		gatewayStatusContributions(translationOutputs, krtopts),
		backendPolicyContributions,
		backendContributions,
		policyTargetContributions,
	}, krtopts.ToOptions("StatusContributions")...)

	s.waitForSync = []cache.InformerSynced{
		s.commonCols.HasSynced,
		finalBackends.HasSynced,
		s.perclientSnapCollection.HasSynced,
		s.mostXdsSnapshots.HasSynced,
		s.statusContributions.HasSynced,
		s.plugins.HasSynced,
		s.translator.HasSynced,
	}

	// contextcheck sees a context.Background() deep in the registration chain, in the slog
	// level check that guards the per-event debug log in statussync.registerResource. A level
	// check takes a context only because slog.Handler.Enabled does; there is nothing to
	// propagate, and pkg/kgateway/setup/controlplane.go checks levels the same way. Threading
	// ctx through four exported statussync registration functions to reach it would be worse.
	s.initStatusInfra(krtopts) //nolint:contextcheck // only ctx use in the chain is a slog level check
}

func (s *ProxySyncer) Start(ctx context.Context) error {
	logger.Info("starting Proxy Syncer", "controller", s.controllerName)
	// Set before the registrations below, which are the gate's only callers.
	s.proxyTranslator.gate.warmWithholdBound = s.commonCols.Settings.PerClientWarmWithholdBound

	// wait for krt collections to sync
	logger.Info("waiting for cache to sync")
	s.apiClient.WaitForCacheSync(
		"kube gw proxy syncer",
		ctx.Done(),
		s.waitForSync...,
	)

	// Note: the controller-runtime manager cache is deliberately not waited on here.
	// Nothing reads through it anymore (status syncing uses the istio/krt cache), so
	// it holds no informers.
	logger.Info("caches warm!")

	// Register after cache sync. The gate publishes only builds that are still
	// the client's current row (beginBuildLocked).
	s.proxyTranslator.gate.currentBuild = func(proxyKey string) *XdsSnapWrapper {
		return s.perclientSnapCollection.GetKey(proxyKey)
	}
	// A client whose snapshot row was already gone when it disconnected gets no
	// row Delete, so end its connection here, unless it has reconnected. When
	// it was its gateway's last client, drop the gateway's scoping series too;
	// otherwise drop only its part of the gateway's scoping state.
	s.uniqueClients.RegisterBatch(func(events []krt.Event[ir.UniquelyConnectedClient]) {
		for _, e := range events {
			if name := e.Latest().ResourceName(); e.Event == controllers.EventDelete && s.uniqueClients.GetKey(name) == nil {
				s.proxyTranslator.gate.clientDeparted(name)
				forgetClusterScopingClient(name, gatewayHasClients(name, s.uniqueClients.List()))
			}
		}
	}, false)

	// Reclaim departed clients' SnapshotCache entries (see snapshotReclaimer).
	if s.clientState != nil {
		reclaimer := newSnapshotReclaimer(func(resourceName string) bool {
			return s.proxyTranslator.gate.reclaimDeparted(s.proxyTranslator.xdsCache, resourceName, s.clientState)
		})
		// A queued build or firing timer can publish after departure and
		// re-create a reclaimed entry, which would then leak. Re-arm the reclaim
		// (a no-op while one is pending); it still checks departure when it fires.
		s.proxyTranslator.gate.afterPublish = func(resourceName string) {
			if s.uniqueClients.GetKey(resourceName) == nil {
				reclaimer.clientDeparted(resourceName)
			}
		}
		// Clear a returning client's old entry before answering it. Registered
		// after afterPublish is set: the server is already serving, and a
		// connection start publishes, which reads it.
		s.proxyTranslator.registerConnectionStartHooks(ctx, s.clientState, func(proxyKey string) *XdsSnapWrapper {
			return s.perclientSnapCollection.GetKey(proxyKey)
		})
		s.uniqueClients.RegisterBatch(func(events []krt.Event[ir.UniquelyConnectedClient]) {
			for _, e := range events {
				name := e.Latest().ResourceName()
				if e.Event == controllers.EventDelete {
					reclaimer.clientDeparted(name)
				} else {
					reclaimer.clientConnected(name)
				}
			}
		}, true)
	}

	s.perclientSnapCollection.RegisterBatch(func(o []krt.Event[XdsSnapWrapper]) {
		for _, e := range o {
			cd := getDetailsFromXDSClientResourceName(e.Latest().ResourceName())

			if e.Event != controllers.EventDelete {
				snapWrap := e.Latest()
				s.proxyTranslator.syncXds(ctx, snapWrap)
			} else {
				// Deferral keeps rows, so a Delete means the client departed or
				// an input briefly vanished. The cache entry stays: clearing it
				// would withdraw Envoy's last coherent snapshot and cause
				// 500/NC on valid routes. Cancel pending bounded publishes so
				// no timer fires for a gone row; a live client re-arms them. A
				// connected client keeps its published state; a departed one
				// starts over if it returns. The snapshotReclaimer above
				// reclaims departed clients' entries.
				name := e.Latest().ResourceName()
				s.proxyTranslator.clientSnapshotDeleted(name, s.uniqueClients.GetKey(name) != nil)
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

// StatusCollections returns the registered desired-status collections. The status syncer
// enables them (attaching handlers that feed the write queue) on the leader.
func (s *ProxySyncer) StatusCollections() *statussync.StatusCollections {
	return s.statusCollections
}

// StatusWriters returns the per-GVK status writers used to persist desired statuses.
func (s *ProxySyncer) StatusWriters() map[schema.GroupVersionKind]statussync.ResourceStatusSyncer {
	return s.statusWriters
}

// StatusContributions returns the independently keyed status facts emitted by translation.
func (s *ProxySyncer) StatusContributions() krt.Collection[reports.StatusContribution] {
	return s.statusContributions
}

// StatusContributionsByTarget returns the index used to reduce facts per status owner.
func (s *ProxySyncer) StatusContributionsByTarget() krt.Index[reports.StatusKey, reports.StatusContribution] {
	return s.statusContributionsByTarget
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
