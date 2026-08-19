package proxy_syncer

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"sync/atomic"

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

	uniqueClients krt.Collection[ir.UniquelyConnectedClient]

	statusContributions         krt.Collection[reports.StatusContribution]
	statusContributionsByTarget krt.Index[reports.StatusKey, reports.StatusContribution]
	gatewayStatusSnapshots      krt.Collection[GatewayStatusSnapshot]
	mostXdsSnapshots            krt.Collection[GatewayXdsResources]
	perclientSnapCollection     krt.Collection[XdsSnapWrapper]

	statusCollections *statussync.StatusCollections
	statusWriters     map[schema.GroupVersionKind]statussync.ResourceStatusSyncer

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
		r.Secrets.Version == in.Secrets.Version
}

// GatewayStatusSnapshot is the status-only projection of one Gateway
// translation. Keeping it separate prevents status-only changes from
// invalidating the xDS projection.
type GatewayStatusSnapshot struct {
	types.NamespacedName
	Contributions []reports.StatusContribution
}

func (r GatewayStatusSnapshot) ResourceName() string {
	return xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, r.Namespace, r.Name)
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

func toTranslationOutput(gw ir.Gateway, xdsSnap irtranslator.TranslationResult, r reports.ReportMap) *gatewayTranslationOutput {
	c, ch := sliceToResourcesHash(xdsSnap.ExtraClusters)
	nn := types.NamespacedName{
		Namespace: gw.Obj.GetNamespace(),
		Name:      gw.Obj.GetName(),
	}
	return &gatewayTranslationOutput{
		Xds: GatewayXdsResources{
			NamespacedName: nn,
			ClustersHash:   ch,
			Clusters:       c,
			Routes:         sliceToResources(xdsSnap.Routes),
			Listeners:      sliceToResources(xdsSnap.Listeners),
			Secrets:        sliceToResources(xdsSnap.Secrets),
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
) *ProxySyncer {
	return &ProxySyncer{
		controllerName:  controllerName,
		commonCols:      commonCols,
		apiClient:       client,
		proxyTranslator: NewProxyTranslator(xdsCache),
		uniqueClients:   uniqueClients,
		translator:      translator.NewCombinedTranslator(ctx, mergedPlugins, commonCols, validator),
		plugins:         mergedPlugins,
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

	translationOutputs := krt.NewCollection(s.commonCols.GatewayIndex.Gateways, func(kctx krt.HandlerContext, gw ir.Gateway) *gatewayTranslationOutput {
		// Note: s.commonCols.GatewayIndex.Gateways is already filtered to only include Gateways
		// with controllerName matching s.controllerName (envoy controller). The filtering happens
		// in GatewaysForEnvoyTransformationFunc in pkg/krtcollections/policy.go
		logger.Debug("building proxy for kube gw", "name", client.ObjectKeyFromObject(gw.Obj), "version", gw.Obj.GetResourceVersion())

		xdsSnap, rm := s.translator.TranslateGateway(kctx, ctx, gw)
		if xdsSnap == nil {
			return nil
		}

		return toTranslationOutput(gw, *xdsSnap, rm)
	}, krtopts.ToOptions("GatewayTranslationOutputs")...)
	s.mostXdsSnapshots = krt.NewCollection(translationOutputs, func(_ krt.HandlerContext, output gatewayTranslationOutput) *GatewayXdsResources {
		return &output.Xds
	}, krtopts.ToOptions("MostXdsSnapshots")...)
	s.gatewayStatusSnapshots = krt.NewCollection(translationOutputs, func(_ krt.HandlerContext, output gatewayTranslationOutput) *GatewayStatusSnapshot {
		return &output.Status
	}, krtopts.ToOptions("GatewayStatusSnapshots")...)

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
		localClusterEpPerClient,
	)

	excludedPolicyKinds := make(map[schema.GroupKind]struct{})
	for gk, plugin := range s.plugins.ContributesPolicies {
		if plugin.PolicyStatusFromGatewayReports {
			excludedPolicyKinds[gk] = struct{}{}
		}
	}

	backendPolicyContributions := backendPolicyStatusContributions(
		finalBackendsWithPolicyStatus,
		excludedPolicyKinds,
		krtopts,
	)

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

	// All status paths now meet as independently keyed contributions. Policy
	// ancestors from Gateway and Backend translation naturally reduce under the
	// same policy key without a competing singleton writer.
	s.statusContributions = krt.JoinCollection([]krt.Collection[reports.StatusContribution]{
		gatewayStatusContributions(s.gatewayStatusSnapshots, krtopts),
		backendPolicyContributions,
		backendContributions,
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

	// caches are warm, now we can do registrations
	s.perclientSnapCollection.RegisterBatch(func(o []krt.Event[XdsSnapWrapper]) {
		for _, e := range o {
			cd := getDetailsFromXDSClientResourceName(e.Latest().ResourceName())

			// Deletes are an intentional no-op. When snapshotPerClient returns
			// nil (its per-client inputs weren't derived yet, so it deferred
			// publishing), KRT surfaces a Delete for this UCC. Clearing the xDS
			// cache here would withdraw Envoy's last coherent Snapshot for the
			// duration of the defer, causing 500/NC on valid routes. Leaving the
			// cache alone means Envoy keeps serving its previously-published
			// config until a new snapshot overwrites it — "retain last good".
			//
			// Known leak: a Delete also arrives when a UCC truly goes away (Envoy
			// pod replaced on rollout, scaled down, etc.), and we cannot
			// distinguish that from the "defer" case here. The SnapshotCache entry
			// for that UCC is therefore never cleared and accumulates over the
			// controller's lifetime. Pre-existing behavior (the prior ClearSnapshot
			// call was already commented out); reclaiming these entries requires a
			// separate signal — e.g. cross-referencing uccCol membership — and is
			// left to a follow-up.
			if e.Event != controllers.EventDelete {
				snapWrap := e.Latest()
				s.proxyTranslator.syncXds(ctx, snapWrap)
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
