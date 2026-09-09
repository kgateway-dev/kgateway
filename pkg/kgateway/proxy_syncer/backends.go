package proxy_syncer

import (
	"cmp"
	"context"
	"hash/fnv"
	"slices"
	"strconv"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// baseEnvoyCluster is the UCC-invariant translation result for a single backend.
// The Cluster proto is shared across every UCC that targets this backend: it is
// read-only on the consumer side, and per-client processing clones it before
// modifying. Sharing it is what keeps per-client CDS state proportional to the
// number of backends rather than to backends times clients.
type baseEnvoyCluster struct {
	// Name is both the Envoy cluster name and the KRT key; translation always
	// names the cluster (blackhole included) after BackendObjectIR.ClusterName().
	Name string
	// Cluster is wrapped so consumers cannot mutate the proto shared across
	// every client snapshot; see package sharedproto. Content equality is
	// carried by ClusterVersion (a content hash over the proto plus, for
	// inline-CLA bases, the endpoint and policy inputs) together with Error.
	// +noKrtEquals
	Cluster        sharedproto.Shared[*envoyclusterv3.Cluster]
	ClusterVersion uint64
	// Error is the translation error for this backend, if any. Compared by message in
	// Equals because all errored clusters share one blackhole proto and baseClusterVersion
	// collapses every error to 0, so ClusterVersion can't tell error states apart.
	Error error
	// BackendSource identifies the Backend this cluster was translated from, for status attribution.
	BackendSource ir.ObjectSource
	// BackendGeneration is the observed generation of the source Backend.
	BackendGeneration int64
	// Backend is the IR this cluster was translated from. Per-client processing
	// and the overlays it runs read it, so it is compared by content: a
	// metadata change to the backing object (a Service label an overlay
	// branches on) then reaches every client even when the shared proto is
	// byte-identical, without a second collection keyed on the raw backend. Its
	// resourceVersion is deliberately not compared: every Service write bumps
	// it, including status and controller annotation touches, and the proto
	// hash above already says whether the translation moved. Comparing it
	// would rerun every client's walk for a write that changed nothing.
	Backend *ir.BackendObjectIR
	// Base is the non-proto portion of the base-translation result retained for
	// per-client processing. Base.Cluster is always nil: the only retained copy
	// of the shared proto lives behind Cluster, so future code cannot mutate it
	// through a raw *BaseCluster alias. Everything ApplyPerClient reads from it
	// (EndpointInputs, SupportsInlineCLA, DefaultedLocalityConfig) is either
	// derived from the proto or folded into ClusterVersion by baseClusterVersion.
	// +noKrtEquals
	Base *irtranslator.BaseCluster
}

func (b baseEnvoyCluster) ResourceName() string { return b.Name }

func (b baseEnvoyCluster) Equals(in baseEnvoyCluster) bool {
	return b.Name == in.Name &&
		b.ClusterVersion == in.ClusterVersion &&
		b.BackendSource == in.BackendSource &&
		b.BackendGeneration == in.BackendGeneration &&
		errString(b.Error) == errString(in.Error) &&
		backendEquals(b.Backend, in.Backend)
}

func backendEquals(a, b *ir.BackendObjectIR) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.EqualsIgnoringResourceVersion(*b)
}

// uccWithCluster is one client's view of one backend's cluster: the shared base
// or this client's own clone, along with any translation error and the source
// Backend identity used for status attribution. clustersForClient returns it,
// and it is also the row type of the status collection (StatusClusters), where
// Cluster and ClusterVersion are left zero because status does not read them.
type uccWithCluster struct {
	Client ir.UniquelyConnectedClient
	// Cluster is wrapped so snapshot assembly cannot mutate a proto shared with
	// other clients; the only exits are ResourceWithTTL (into the envoycache
	// snapshot, tripwire-verified) and Clone. Content equality is carried by
	// ClusterVersion, a content hash over the same proto.
	// +noKrtEquals
	Cluster        sharedproto.Shared[*envoyclusterv3.Cluster]
	ClusterVersion uint64
	Name           string
	Error          error
	// PerClientError reports that Error was produced for this client alone (a
	// strict-mode validation failure of an overlaid cluster) rather than by the
	// shared base translation. Status attributes base errors once, from the base
	// row, and per-client errors per client; this is how the two are told apart.
	PerClientError bool
	// BackendSource identifies the Backend this cluster was translated from, for status attribution.
	BackendSource ir.ObjectSource
	// BackendGeneration is the observed generation of the source Backend.
	BackendGeneration int64
}

// uccClusterResourceName builds the per-client identity key for a uccWithCluster
// row. Plain concatenation is ~2.5x cheaper than fmt.Sprintf on these key shapes
// and allocates once instead of three times.
func uccClusterResourceName(client ir.UniquelyConnectedClient, name string) string {
	return client.ResourceName() + "/" + name
}

func (c uccWithCluster) ResourceName() string {
	return uccClusterResourceName(c.Client, c.Name)
}

func (c uccWithCluster) Equals(in uccWithCluster) bool {
	return c.Client.Equals(in.Client) &&
		c.ClusterVersion == in.ClusterVersion &&
		c.Name == in.Name &&
		c.PerClientError == in.PerClientError &&
		c.BackendSource == in.BackendSource &&
		c.BackendGeneration == in.BackendGeneration &&
		errString(c.Error) == errString(in.Error)
}

// errString renders an error for comparison inside an Equals method, where a nil
// error and an empty message must compare equal.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// clustersWithErrors is one client's assembled CDS payload plus the clusters that
// failed to translate for that client. Errored clusters are deliberately kept out
// of the payload but tracked by name, because a route pointing at one must return
// a direct response rather than silently falling through to a cluster that isn't
// there.
//
// The hashes are the equality keys for the payload: publishable clusters are
// versioned by content, errored ones only by name, because a cluster Envoy never
// sees need not republish when its error message changes. Per-client errors are
// compared as records, because the status projection built from this row reads
// their source Backend and generation as well as their message: a backend whose
// next generation fails with the same message must still produce a new row, or
// status would report the new generation as accepted while CDS still excludes it.
type clustersWithErrors struct {
	// +noKrtEquals
	clusters envoycache.Resources
	// +noKrtEquals
	erroredClusters     []string
	erroredClustersHash uint64
	clustersHash        uint64
	// perClientErrors are the rows whose Error was produced for this client
	// alone, sorted by cluster name; the status projection reads them. Base
	// errors are attributed from the base rows instead, once rather than once
	// per client.
	perClientErrors []uccWithCluster
	resourceName    string
}

func (c clustersWithErrors) ResourceName() string {
	return c.resourceName
}

var _ krt.Equaler[clustersWithErrors] = new(clustersWithErrors)

func (c clustersWithErrors) Equals(k clustersWithErrors) bool {
	return c.clustersHash == k.clustersHash &&
		c.erroredClustersHash == k.erroredClustersHash &&
		c.resourceName == k.resourceName &&
		slices.EqualFunc(c.perClientErrors, k.perClientErrors, uccWithCluster.Equals)
}

// baseClusterVersion returns the equality hash for a base translation result.
// It folds the inline endpoints hash into the cluster proto hash when the cluster
// type supports an inline CLA: the per-client CLA is built from
// BaseCluster.EndpointInputs and is NOT part of the base proto, so without this
// the base would not re-publish when endpoints change, leaving clients pinned
// to stale LoadAssignments for non-EDS backends (e.g. ServiceEntry-style).
//
// The attached-policy hash is folded in for the same reason: EndpointInputs
// carries the backend's AttachedPolicies, which per-client endpoint hooks consume
// when building the per-client CLA. KRT keeps the OLD stored object when Equals
// returns true, so any Base state consumed downstream but missing from this
// version would be served stale forever. This mirrors the EDS path, which folds
// backendEndpointVersionHash into LbEpsEqualityHash in newFinalBackendEndpoints.
//
// For EDS clusters EndpointInputs may also be non-nil, but those endpoints feed
// the separate EDS pipeline and are not used by ApplyPerClient; gating on
// SupportsInlineCLA keeps the version stable for the EDS case so equivalent
// translations do not churn the snapshot.
func baseClusterVersion(backend *ir.BackendObjectIR, b *irtranslator.BaseCluster) uint64 {
	if b.Error != nil {
		return 0
	}
	hasher := fnv.New64a()
	utils.HashProtoWithHasher(hasher, b.Cluster)
	if b.SupportsInlineCLA && b.EndpointInputs != nil {
		utils.HashUint64(hasher, b.EndpointInputs.EndpointsForBackend.LbEpsEqualityHash)
		utils.HashUint64(hasher, backendEndpointVersionHash(backend))
	}
	return hasher.Sum64()
}

// PerClientEnvoyClusters is the cluster half of per-client xDS:
//
//   - base holds the UCC-invariant translation, one row per backend, whose Cluster
//     proto is shared read-only by every client that targets it.
//   - perClient holds one row per connected client: that client's assembled CDS
//     payload, built by walking the bases and applying the client's overlays in
//     place. It is the only per-client state, and it is written by exactly one
//     transform, so there is nothing to reconcile against the base collection.
//   - status projects the errors out for Backend status: base errors once, from
//     the base rows, and per-client errors from the per-client rows.
//
// Consumers never read the fields directly: snapshotPerClient fetches perClient
// by client key, and StatusClusters returns the status projection. Construct
// with [NewPerClientEnvoyClusters].
type PerClientEnvoyClusters struct {
	base      krt.Collection[baseEnvoyCluster]
	perClient krt.Collection[clustersWithErrors]
	// status is built once by the constructor. Deriving it on demand instead
	// would let a second caller stand up a duplicate collection over the same
	// inputs, which KRT has no way to flag.
	status krt.Collection[uccWithCluster]
}

// HasSynced reports whether the base and per-client collections have synced.
// Publishing is not gated on this (the readiness gates were reverted in favor
// of the first-connect grace period, #14380); it exists for callers, currently
// tests, that need to wait for cluster translation to reach steady state.
func (iu *PerClientEnvoyClusters) HasSynced() bool {
	if iu.base != nil && !iu.base.HasSynced() {
		return false
	}
	if iu.perClient != nil && !iu.perClient.HasSynced() {
		return false
	}
	return true
}

// clustersForClient is the per-client walk over the base collection: one
// client's view of every base cluster, the shared proto where no overlay applies
// and this client's own clone where one does, along with any error. Every base
// is either published as-is or cloned for this client by ApplyPerClient, so the
// result is complete by construction: a client can never observe a base whose
// overlay has not been applied, nor an inline-CLA cluster without its CLA,
// because both are produced here, before anything is returned.
//
// It is called only by the perClient transform, which stores its result;
// production consumers read that stored row. Tests that want the merge without
// KRT call it directly with a dummy context.
//
// The *Cluster protos in the returned slice are shared with other UCCs (base)
// or unique to this UCC (clone); callers MUST NOT mutate them.
func clustersForClient(
	kctx krt.HandlerContext,
	ctx context.Context,
	translator *irtranslator.BackendTranslator,
	base krt.Collection[baseEnvoyCluster],
	ucc ir.UniquelyConnectedClient,
) []uccWithCluster {
	bases := krt.Fetch(kctx, base)
	out := make([]uccWithCluster, 0, len(bases))
	for _, b := range bases {
		row := uccWithCluster{
			Client:            ucc,
			Cluster:           b.Cluster,
			ClusterVersion:    b.ClusterVersion,
			Name:              b.Name,
			Error:             b.Error,
			BackendSource:     b.BackendSource,
			BackendGeneration: b.BackendGeneration,
		}
		// An errored base is the same blackhole for every client, and a base
		// built without its translation inputs (test fixtures) has nothing to
		// overlay; both publish as-is.
		if b.Error != nil || translator == nil || b.Base == nil || b.Backend == nil {
			out = append(out, row)
			continue
		}
		// Lend ApplyPerClient the shared base proto rather than a copy of it. It
		// only reads this proto, and clones internally before letting an overlay
		// touch one, so it already performs the single clone a per-client
		// cluster needs, and none at all on the dominant path where it returns
		// nil untouched.
		perClientBase := *b.Base
		perClientBase.Cluster = b.Cluster.BorrowForRead()
		perClient, err := translator.ApplyPerClient(kctx, ctx, ucc, b.Backend, &perClientBase)
		switch {
		case err != nil:
			// Carry the error so the snapshot tracks this cluster as errored for
			// THIS client only. Falling back to the (valid) base would defeat
			// strict-mode validation; the user opted in to having broken configs
			// surface as errors rather than NACKs at the Envoy data plane.
			logger.Error("failed to apply per-client overlay",
				"backend", b.Name, "ucc", ucc.ResourceName(), "error", err)
			if perClient != nil {
				row.Name = perClient.GetName()
			}
			// Hash 0: errored rows are never published, so they opt out of
			// tripwire verification.
			row.Cluster = sharedproto.WrapPrehashed(perClient, 0)
			row.ClusterVersion = utils.HashString(err.Error())
			row.Error = err
			row.PerClientError = true
		case perClient != nil:
			clusterVersion := utils.HashProto(perClient)
			row.Name = perClient.GetName()
			row.Cluster = sharedproto.WrapPrehashed(perClient, clusterVersion)
			row.ClusterVersion = clusterVersion
		}
		out = append(out, row)
	}
	return out
}

// assemblePerClientClusters turns one client's merged view into the CDS payload
// snapshotPerClient publishes. Errored clusters are excluded from the payload
// and tracked by name; per-client errors are additionally retained in full for
// the status projection.
func assemblePerClientClusters(ucc ir.UniquelyConnectedClient, rows []uccWithCluster) *clustersWithErrors {
	clustersProto := make([]envoycachetypes.ResourceWithTTL, 0, len(rows))
	var (
		clustersHash        uint64
		erroredClustersHash uint64
		erroredClusters     []string
		perClientErrors     []uccWithCluster
	)
	for _, c := range rows {
		if c.Error != nil {
			erroredClusters = append(erroredClusters, c.Name)
			// For errored clusters, we don't want to include the cluster version
			// in the hash. The cluster version is the hash of the proto. because this cluster
			// won't be sent to envoy anyway, there's no point trigger updates if it changes from
			// one error state to a different error state.
			erroredClustersHash ^= utils.HashString(c.Name)
			if c.PerClientError {
				perClientErrors = append(perClientErrors, c)
			}
			continue
		}
		// ResourceWithTTL is the only exit for the shared proto; it runs the
		// mutation tripwire when armed. See package sharedproto.
		clustersProto = append(clustersProto, c.Cluster.ResourceWithTTL())
		clustersHash ^= c.ClusterVersion
	}
	clustersVersion := strconv.FormatUint(clustersHash, 10)
	// Base rows arrive in map order; sort so Equals compares like with like.
	slices.SortFunc(perClientErrors, func(a, b uccWithCluster) int { return cmp.Compare(a.Name, b.Name) })

	return &clustersWithErrors{
		clusters:            envoycache.NewResourcesWithTTL(clustersVersion, clustersProto),
		erroredClusters:     erroredClusters,
		clustersHash:        clustersHash,
		erroredClustersHash: erroredClustersHash,
		perClientErrors:     perClientErrors,
		resourceName:        ucc.ResourceName(),
	}
}

// StatusClusters returns the status projection built by
// [NewPerClientEnvoyClusters]; see newStatusClusters for what it contains. The
// zero PerClientEnvoyClusters has none.
func (iu *PerClientEnvoyClusters) StatusClusters() krt.Collection[uccWithCluster] {
	return iu.status
}

// newStatusClusters builds the cluster view needed for fleet-wide Backend status
// attribution: one row per base cluster (carrying the source Backend identity and
// any UCC-invariant translation error) plus one row per errored per-client cluster
// (carrying the per-client translation error attributed to the same Backend). Only
// Name, Error, BackendSource, BackendGeneration, and Client on per-client rows, are
// populated; those are the fields GenerateBackendStatusReport consumes. Clusters
// that translated cleanly for a client contribute nothing beyond their base row.
//
// This is a collection rather than a Fetch helper because backendStatusContributions
// indexes it by Backend: one client's cluster error then recomputes only its owning
// Backend's status, not every Backend's.
//
// The two halves are independent projections joined by key. Base rows carry the
// zero UCC, whose ResourceName is empty; a connected client's never is, so the
// halves cannot collide.
func newStatusClusters(
	krtopts krtutil.KrtOptions,
	base krt.Collection[baseEnvoyCluster],
	perClient krt.Collection[clustersWithErrors],
) krt.Collection[uccWithCluster] {
	if base == nil {
		return krt.NewStaticCollection[uccWithCluster](nil, nil, krtopts.ToOptions("BackendStatusClusters")...)
	}
	baseStatus := krt.NewCollection(base, func(_ krt.HandlerContext, b baseEnvoyCluster) *uccWithCluster {
		return &uccWithCluster{
			Name:              b.Name,
			Error:             b.Error,
			BackendSource:     b.BackendSource,
			BackendGeneration: b.BackendGeneration,
		}
	}, krtopts.ToOptions("BackendStatusClustersBase")...)
	perClientErrors := krt.NewManyCollection(perClient, func(_ krt.HandlerContext, c clustersWithErrors) []uccWithCluster {
		return c.perClientErrors
	}, krtopts.ToOptions("BackendStatusClustersPerClient")...)
	return krt.JoinCollection(
		[]krt.Collection[uccWithCluster]{baseStatus, perClientErrors},
		append(krtopts.ToOptions("BackendStatusClusters"), krt.WithJoinUnchecked())...,
	)
}

// NewPerClientEnvoyClusters builds the collections that back
// [PerClientEnvoyClusters], translating every backend in finalBackends into a
// shared base cluster once, and assembling every client in uccs a CDS payload
// from those bases plus its own overlays.
//
// The work is split so that the expensive part does not scale with the client
// count: each backend is translated once into a shared base, and each connected
// client is then offered a cheap overlay on top of it. Only the (client, backend)
// pairs whose cluster genuinely differs (a matching destination rule, a waypoint
// redirect, an inline CLA, a per-client validation failure) allocate anything per
// client. For a fleet where few backends vary per client, retained state stays
// close to O(backends) plus one payload per client.
//
// A client's payload depends on the base collection and on whatever its overlays
// fetch, and on nothing else: another client connecting, disconnecting, or
// changing shape does not rerun it.
func NewPerClientEnvoyClusters(
	ctx context.Context,
	krtopts krtutil.KrtOptions,
	translator *irtranslator.BackendTranslator,
	finalBackends krt.Collection[*ir.BackendObjectIR],
	uccs krt.Collection[ir.UniquelyConnectedClient],
) PerClientEnvoyClusters {
	// Base clusters: one entry per backend, computed once and shared across all
	// UCCs. Anything that does not depend on the UCC lives here:
	// initializeCluster, InitEnvoyBackend, DNS lookup family, non-per-client
	// ProcessBackend hooks, gateway client certificate injection, and strict-mode
	// validation.
	base := krt.NewCollection(finalBackends, func(kctx krt.HandlerContext, backendObj *ir.BackendObjectIR) *baseEnvoyCluster {
		baseRes := translator.TranslateBackendBase(kctx, ctx, backendObj)
		if baseRes == nil {
			return nil
		}
		name := baseRes.Cluster.GetName()
		if name != backendObj.ClusterName() {
			// Every consumer assumes the cluster is named after the backend's
			// memoized ClusterName: routes reference it, status is keyed on it,
			// and the EDS pipeline names its CLA after it. Nothing in tree
			// renames the cluster (initializeCluster and buildBlackholeCluster
			// both take the name from ClusterName, and no plugin reassigns it);
			// if that changes, drop this one backend loudly rather than publish
			// a cluster nothing can reach.
			logger.Error("backend translation renamed the cluster; dropping the backend",
				"backend", backendObj.ResourceName(),
				"expected", backendObj.ClusterName(), "got", name)
			return nil
		}
		clusterVersion := baseClusterVersion(backendObj, baseRes)
		sharedCluster := sharedproto.Wrap(baseRes.Cluster)
		// Seal the only retained raw alias. Per-client processing reconstructs a
		// temporary BaseCluster whose Cluster is borrowed from sharedCluster.
		baseRes.Cluster = nil
		var backendGeneration int64
		if backendObj.Obj != nil {
			backendGeneration = backendObj.Obj.GetGeneration()
		}
		return &baseEnvoyCluster{
			Name:              name,
			Cluster:           sharedCluster,
			ClusterVersion:    clusterVersion,
			Error:             baseRes.Error,
			BackendSource:     backendObj.GetObjectSource(),
			BackendGeneration: backendGeneration,
			Backend:           backendObj,
			Base:              baseRes,
		}
	}, krtopts.ToOptions("BaseEnvoyClusters")...)

	return newPerClientEnvoyClusters(ctx, krtopts, translator, base, uccs)
}

// newPerClientEnvoyClusters builds the per-client and status collections over an
// existing base collection. Tests that need a specific set of base rows use it
// directly.
func newPerClientEnvoyClusters(
	ctx context.Context,
	krtopts krtutil.KrtOptions,
	translator *irtranslator.BackendTranslator,
	base krt.Collection[baseEnvoyCluster],
	uccs krt.Collection[ir.UniquelyConnectedClient],
) PerClientEnvoyClusters {
	// Per-client payloads: one row per connected client, keyed by the client.
	// This transform is the only writer of per-client cluster state, and it
	// reads only immutable base rows plus whatever the client's overlays fetch,
	// so it needs no fence against any other collection. The dominant path
	// through it allocates nothing per backend: the shared base proto is
	// published as-is.
	perClient := krt.NewCollection(uccs, func(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) *clustersWithErrors {
		rows := clustersForClient(kctx, ctx, translator, base, ucc)
		if len(rows) == 0 {
			// No backends translated yet. Unreachable in a real cluster, where
			// kubernetes.default alone is a backend; common in tests. The
			// snapshot stage treats the missing row as "not ready" and keeps
			// the client's last snapshot.
			logger.Debug("no base clusters; deferring per-client clusters", "client", ucc.ResourceName())
			return nil
		}
		return assemblePerClientClusters(ucc, rows)
	}, krtopts.ToOptions("ClusterResources")...)

	return PerClientEnvoyClusters{
		base:      base,
		perClient: perClient,
		status:    newStatusClusters(krtopts, base, perClient),
	}
}
