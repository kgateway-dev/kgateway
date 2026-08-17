package proxy_syncer

import (
	"cmp"
	"context"
	"hash/fnv"
	"slices"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// baseEnvoyCluster is the UCC-invariant translation result for a single backend.
// The Cluster proto is shared across every UCC that targets this backend — it is
// read-only on the consumer side, and per-client mutations clone it before
// modifying. This is the change that lets the per-client collection stay sparse.
type baseEnvoyCluster struct {
	// Name is both the Envoy cluster name and the KRT key; translation always
	// names the cluster (blackhole included) after BackendObjectIR.ClusterName(),
	// which is how the deltas builder looks bases up.
	Name string
	// Cluster is wrapped so consumers cannot mutate the proto shared across
	// every client snapshot; see package sharedproto. Content equality is
	// carried by ClusterVersion (a content hash over the proto plus, for
	// inline-CLA bases, the endpoint and policy inputs) together with Error.
	// +noKrtEquals
	Cluster        sharedproto.Shared[*envoyclusterv3.Cluster]
	ClusterVersion uint64
	// Fingerprint fences per-client deltas to the exact base/backend input they
	// were computed from. It intentionally includes metadata and policy inputs
	// that may affect an overlay without changing the base Cluster proto.
	Fingerprint baseClusterFingerprint
	// Error is the translation error for this backend, if any. Compared by message in
	// Equals because all errored clusters share one blackhole proto and baseClusterVersion
	// collapses every error to 0, so ClusterVersion can't tell error states apart.
	Error error
	// BackendSource identifies the Backend this cluster was translated from, for status attribution.
	BackendSource ir.ObjectSource
	// BackendGeneration is the observed generation of the source Backend.
	BackendGeneration int64
	// NeedsInlineCLA is captured before Base.Cluster is sealed below.
	NeedsInlineCLA bool
	// Base is the non-proto portion of the base-translation result retained for
	// per-client processing. Base.Cluster is always nil: the only retained copy
	// of the shared proto lives behind Cluster, so future code cannot mutate it
	// through a raw *BaseCluster alias.
	// +noKrtEquals
	Base *irtranslator.BaseCluster
}

func (b baseEnvoyCluster) ResourceName() string { return b.Name }

func (b baseEnvoyCluster) Equals(in baseEnvoyCluster) bool {
	return b.Name == in.Name &&
		b.ClusterVersion == in.ClusterVersion &&
		b.Fingerprint == in.Fingerprint &&
		b.BackendSource == in.BackendSource &&
		b.BackendGeneration == in.BackendGeneration &&
		b.NeedsInlineCLA == in.NeedsInlineCLA &&
		errString(b.Error) == errString(in.Error)
}

// backendInputFingerprint captures the backend inputs that can change a
// per-client overlay while leaving the shared base Cluster byte-identical.
// ResourceVersion covers source-object changes; explicit metadata hashes also
// cover synthetic/test objects that omit it; PolicyVersion covers attachment
// changes sourced independently from the backend object.
type backendInputFingerprint struct {
	UID             string
	ResourceVersion string
	Generation      int64
	Labels          uint64
	Annotations     uint64
	PolicyVersion   uint64
}

type baseClusterFingerprint struct {
	Input          backendInputFingerprint
	ClusterVersion uint64
}

func fingerprintBackendInput(backend *ir.BackendObjectIR) backendInputFingerprint {
	if backend == nil {
		return backendInputFingerprint{}
	}
	fingerprint := backendInputFingerprint{PolicyVersion: backendEndpointVersionHash(backend)}
	if backend.Obj == nil {
		return fingerprint
	}
	fingerprint.UID = string(backend.Obj.GetUID())
	fingerprint.ResourceVersion = backend.Obj.GetResourceVersion()
	fingerprint.Generation = backend.Obj.GetGeneration()
	fingerprint.Labels = utils.HashLabels(backend.Obj.GetLabels())
	fingerprint.Annotations = utils.HashLabels(backend.Obj.GetAnnotations())
	return fingerprint
}

func fingerprintBase(backend *ir.BackendObjectIR, clusterVersion uint64) baseClusterFingerprint {
	return baseClusterFingerprint{
		Input:          fingerprintBackendInput(backend),
		ClusterVersion: clusterVersion,
	}
}

// uccClusterDelta is a per-client cluster materialized only when at least one
// PerClientClusterOverlay returns non-nil for (ucc, backend), when the cluster
// needs an inline CLA (which is always per-client via PrioritizeEndpoints), or
// when strict-mode validation fails on the per-client cluster (the delta then
// carries the blackhole + error so the snapshot tracks it as errored for this
// UCC only — other UCCs may still see a valid cluster).
//
// The containing backendClusterDeltaSet omits entries for the dominant case
// where no overlay applies. This keeps actual delta storage O(N*K), where K is
// the count of backends that genuinely vary per UCC, while one small resolution
// row per backend disambiguates sparse absence.
type uccClusterDelta struct {
	Client ir.UniquelyConnectedClient
	Name   string
	// BaseFingerprint identifies the exact base/backend input this delta was
	// cloned from. A mismatched delta is pending/stale and must never override a
	// newer base.
	BaseFingerprint baseClusterFingerprint
	// Cluster is wrapped so consumers cannot mutate the proto interned across
	// UCCs; see package sharedproto. Content equality is carried by
	// ClusterVersion (the proto content hash; errored rows hash the error
	// message instead) together with Error.
	// +noKrtEquals
	Cluster        sharedproto.Shared[*envoyclusterv3.Cluster]
	ClusterVersion uint64
	// Error participates in Equals by message.
	Error error
}

func (d uccClusterDelta) ResourceName() string {
	return uccClusterResourceName(d.Client, d.Name)
}

func (d uccClusterDelta) Equals(in uccClusterDelta) bool {
	return d.Client.Equals(in.Client) &&
		d.Name == in.Name &&
		d.BaseFingerprint == in.BaseFingerprint &&
		d.ClusterVersion == in.ClusterVersion &&
		errString(d.Error) == errString(in.Error)
}

// uccClusterResourceName builds the per-client identity key for a cluster.
// Deltas are not KRT rows — they live in a map keyed by UCC name inside
// backendClusterDeltaSet — and the uccWithCluster rows that are (the status
// collection) are rebuilt per event, so there is nothing to cache the key on the
// way UccWithEndpoints does. Plain concatenation is still ~2.5x cheaper than
// fmt.Sprintf on these key shapes and allocates once instead of three times.
func uccClusterResourceName(client ir.UniquelyConnectedClient, name string) string {
	return client.ResourceName() + "/" + name
}

// clientSetFingerprint versions the complete UCC input consumed while a
// backend's sparse delta set was evaluated. It lets a newly connected or
// changed client distinguish an explicitly resolved no-overlay result from an
// older delta set that simply never evaluated that client.
type clientSetFingerprint uint64

func fingerprintClients(clients []ir.UniquelyConnectedClient) clientSetFingerprint {
	ordered := slices.Clone(clients)
	slices.SortFunc(ordered, func(a, b ir.UniquelyConnectedClient) int {
		return cmp.Compare(a.ResourceName(), b.ResourceName())
	})
	hasher := fnv.New64a()
	for _, client := range ordered {
		utils.HashStringField(hasher, client.ResourceName())
		utils.HashStringField(hasher, client.Role)
		utils.HashStringField(hasher, client.Namespace)
		utils.HashStringField(hasher, client.Locality.Region)
		utils.HashStringField(hasher, client.Locality.Zone)
		utils.HashStringField(hasher, client.Locality.Subzone)
		utils.HashUint64(hasher, utils.HashLabels(client.Labels))
	}
	return clientSetFingerprint(hasher.Sum64())
}

// backendClusterDeltaSet is the atomic sparse overlay result for one backend.
// A row exists even when Deltas is empty, making absence inside the map mean
// "resolved with no overlay". Absence of the row, or a fingerprint mismatch,
// means "pending" and causes the merged cluster view to defer publication.
type backendClusterDeltaSet struct {
	Name               string
	BaseFingerprint    baseClusterFingerprint
	ClientsFingerprint clientSetFingerprint
	// Deltas contains only clients whose cluster genuinely differs from base.
	// +noKrtEquals
	Deltas map[string]uccClusterDelta
}

func (d backendClusterDeltaSet) ResourceName() string { return d.Name }

func (d backendClusterDeltaSet) Equals(in backendClusterDeltaSet) bool {
	if d.Name != in.Name ||
		d.BaseFingerprint != in.BaseFingerprint ||
		d.ClientsFingerprint != in.ClientsFingerprint ||
		len(d.Deltas) != len(in.Deltas) {
		return false
	}
	for client, delta := range d.Deltas {
		other, ok := in.Deltas[client]
		if !ok || !delta.Equals(other) {
			return false
		}
	}
	return true
}

// uccWithCluster is the merged view returned by FetchClustersForClient: the
// resolved cluster (base or delta) along with any translation error and the
// source Backend identity used for status attribution. It is also the row type
// of the status-only collection built by StatusClusters, where the Cluster and
// ClusterVersion fields are left zero because status does not read them.
type uccWithCluster struct {
	Client ir.UniquelyConnectedClient
	// Cluster is wrapped so snapshot assembly cannot mutate the proto shared
	// with other clients; the only exits are ResourceWithTTL (into the
	// envoycache snapshot, tripwire-verified) and Clone. Content equality is
	// carried by ClusterVersion, a content hash over the same proto.
	// +noKrtEquals
	Cluster        sharedproto.Shared[*envoyclusterv3.Cluster]
	ClusterVersion uint64
	Name           string
	Error          error
	// BackendSource identifies the Backend this cluster was translated from, for status attribution.
	BackendSource ir.ObjectSource
	// BackendGeneration is the observed generation of the source Backend.
	BackendGeneration int64
}

func (c uccWithCluster) ResourceName() string {
	return uccClusterResourceName(c.Client, c.Name)
}

func (c uccWithCluster) Equals(in uccWithCluster) bool {
	return c.Client.Equals(in.Client) &&
		c.ClusterVersion == in.ClusterVersion &&
		c.Name == in.Name &&
		c.BackendSource == in.BackendSource &&
		c.BackendGeneration == in.BackendGeneration &&
		errString(c.Error) == errString(in.Error)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// baseClusterVersion returns the equality hash for a base translation result.
// It folds the inline endpoints hash into the cluster proto hash when the cluster
// type supports an inline CLA: the per-client CLA is built from
// BaseCluster.EndpointInputs and is NOT part of the base proto, so without this
// the base would not re-publish when endpoints change — leaving clients pinned
// to stale LoadAssignments for non-EDS backends (e.g. ServiceEntry-style).
//
// The attached-policy hash is folded in for the same reason: EndpointInputs
// carries the backend's AttachedPolicies, which PerClientProcessEndpoints hooks
// consume when building the per-client CLA. KRT keeps the OLD stored object when
// Equals returns true, so any Base state consumed downstream but missing from
// this version would be served stale forever. This mirrors the EDS path, which
// folds backendEndpointVersionHash into LbEpsEqualityHash in
// newFinalBackendEndpoints.
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

type PerClientEnvoyClusters struct {
	base    krt.Collection[baseEnvoyCluster]
	deltas  krt.Collection[backendClusterDeltaSet]
	clients krt.Collection[ir.UniquelyConnectedClient]
}

// HasSynced reports whether both the base and delta collections have synced.
// Publishing is not gated on this (the readiness gates were reverted in favor
// of the first-connect grace period, #14380); it exists for callers — currently
// tests — that need to wait for cluster translation to reach steady state.
func (iu *PerClientEnvoyClusters) HasSynced() bool {
	if iu.base != nil && !iu.base.HasSynced() {
		return false
	}
	if iu.deltas != nil && !iu.deltas.HasSynced() {
		return false
	}
	return true
}

// FetchClustersForClient returns the merged set of clusters for a UCC: a
// per-client delta for each backend that has one, and the shared base cluster
// otherwise. Before returning anything it verifies that every backend's atomic
// delta set was evaluated against the current base and client set. A mismatch
// returns no rows, causing snapshot assembly to retain the last coherent xDS
// snapshot while the dependent KRT transforms catch up.
//
// The *Cluster protos in the returned slice are shared with other UCCs (base)
// or unique to this UCC (delta); callers MUST NOT mutate them.
func (iu *PerClientEnvoyClusters) FetchClustersForClient(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) []uccWithCluster {
	var bases []baseEnvoyCluster
	if iu.base != nil {
		bases = krt.Fetch(kctx, iu.base)
	}
	var deltaSets []backendClusterDeltaSet
	if iu.deltas != nil {
		deltaSets = krt.Fetch(kctx, iu.deltas)
	}

	deltaSetByName := make(map[string]*backendClusterDeltaSet, len(deltaSets))
	for i := range deltaSets {
		deltaSetByName[deltaSets[i].Name] = &deltaSets[i]
	}

	var clientsFingerprint clientSetFingerprint
	if iu.clients != nil {
		clients := krt.Fetch(kctx, iu.clients)
		clientsFingerprint = fingerprintClients(clients)
		clientIsCurrent := false
		for _, current := range clients {
			if current.Equals(ucc) {
				clientIsCurrent = true
				break
			}
		}
		if !clientIsCurrent {
			return nil
		}
	}

	// Validate the entire generation before exposing any row. Returning a
	// partial base/delta mix would let snapshotPerClient publish an incoherent
	// CDS payload while collections converge.
	for _, b := range bases {
		set, ok := deltaSetByName[b.Name]
		if !ok || set.BaseFingerprint != b.Fingerprint {
			return nil
		}
		if iu.clients != nil && set.ClientsFingerprint != clientsFingerprint {
			return nil
		}
		if d, ok := set.Deltas[ucc.ResourceName()]; ok {
			if !d.Client.Equals(ucc) || d.BaseFingerprint != b.Fingerprint {
				return nil
			}
		} else if b.NeedsInlineCLA {
			// Inline-CLA bases must always materialize a per-client delta.
			return nil
		}
	}

	out := make([]uccWithCluster, 0, len(bases))
	for _, b := range bases {
		set := deltaSetByName[b.Name]
		if d, ok := set.Deltas[ucc.ResourceName()]; ok {
			// Delta wins on cluster + version. Delta error wins over base error
			// because a per-UCC failure (e.g. strict-mode validation of the
			// post-overlay cluster) is the more specific signal — base errors
			// are caught by the short-circuit in the deltas builder, so reaching
			// this branch with a base.Error set is impossible in production.
			// Backend identity always comes from the base, which is where the
			// source Backend is tracked.
			derr := d.Error
			if derr == nil {
				derr = b.Error
			}
			out = append(out, uccWithCluster{
				Client:            ucc,
				Cluster:           d.Cluster,
				ClusterVersion:    d.ClusterVersion,
				Name:              d.Name,
				Error:             derr,
				BackendSource:     b.BackendSource,
				BackendGeneration: b.BackendGeneration,
			})
			continue
		}
		out = append(out, uccWithCluster{
			Client:            ucc,
			Cluster:           b.Cluster,
			ClusterVersion:    b.ClusterVersion,
			Name:              b.Name,
			Error:             b.Error,
			BackendSource:     b.BackendSource,
			BackendGeneration: b.BackendGeneration,
		})
	}
	return out
}

// StatusClusters is the cluster view needed for fleet-wide Backend status
// attribution: one row per base cluster (carrying the source Backend identity and
// any UCC-invariant translation error) plus one row per errored per-client delta
// (carrying the per-client translation error attributed to the same Backend). Only
// Name, Error, BackendSource, BackendGeneration — and Client on delta rows — are
// populated; those are the fields GenerateBackendStatusReport consumes. Non-errored
// deltas contribute nothing to status and are skipped.
//
// This is a collection rather than a Fetch helper because backendStatusContributions
// indexes it by Backend: one client's cluster error then recomputes only its owning
// Backend's status, not every Backend's.
//
// Unlike FetchClustersForClient, a stale delta set is skipped rather than treated as
// a barrier. Status has no cross-backend coherence requirement, and withholding every
// row mid-propagation would clear Accepted conditions that are still true. The
// client-set fingerprint is deliberately not re-checked here: recomputing it would
// cost O(backends*clients) hashing per event, and a delta set whose client set moved
// is recomputed anyway (ClientsFingerprint participates in its Equals), so at worst a
// departed client's error lingers for one propagation.
func (iu *PerClientEnvoyClusters) StatusClusters(krtopts krtutil.KrtOptions) krt.Collection[uccWithCluster] {
	if iu.base == nil {
		return krt.NewStaticCollection[uccWithCluster](nil, nil, krtopts.ToOptions("BackendStatusClusters")...)
	}
	return krt.NewManyCollection(iu.base, func(kctx krt.HandlerContext, b baseEnvoyCluster) []uccWithCluster {
		// The base row carries the zero UCC, whose ResourceName is empty; a connected
		// client's never is, so base and delta rows cannot collide on the KRT key.
		out := []uccWithCluster{{
			Name:              b.Name,
			Error:             b.Error,
			BackendSource:     b.BackendSource,
			BackendGeneration: b.BackendGeneration,
		}}
		if iu.deltas == nil {
			return out
		}
		// The delta set's KRT key is its backend's cluster name, so a keyed FetchOne
		// is both the narrowest dependency and cheaper than a secondary index.
		set := krt.FetchOne(kctx, iu.deltas, krt.FilterKey(b.Name))
		if set == nil || set.BaseFingerprint != b.Fingerprint {
			return out
		}
		for _, d := range set.Deltas {
			if d.Error == nil || d.BaseFingerprint != b.Fingerprint {
				continue
			}
			out = append(out, uccWithCluster{
				Client:            d.Client,
				Name:              d.Name,
				Error:             d.Error,
				BackendSource:     b.BackendSource,
				BackendGeneration: b.BackendGeneration,
			})
		}
		return out
	}, krtopts.ToOptions("BackendStatusClusters")...)
}

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
		baseRes := translator.TranslateBackendBase(ctx, backendObj)
		if baseRes == nil {
			return nil
		}
		clusterVersion := baseClusterVersion(backendObj, baseRes)
		needsInlineCLA := baseRes.NeedsInlineCLA()
		name := baseRes.Cluster.GetName()
		sharedCluster := sharedproto.Wrap(baseRes.Cluster)
		// Seal the only retained raw alias. Per-client processing reconstructs a
		// temporary BaseCluster whose Cluster is cloned from sharedCluster.
		baseRes.Cluster = nil
		var backendGeneration int64
		if backendObj.Obj != nil {
			backendGeneration = backendObj.Obj.GetGeneration()
		}
		return &baseEnvoyCluster{
			Name:              name,
			Cluster:           sharedCluster,
			ClusterVersion:    clusterVersion,
			Fingerprint:       fingerprintBase(backendObj, clusterVersion),
			Error:             baseRes.Error,
			BackendSource:     backendObj.GetObjectSource(),
			BackendGeneration: backendGeneration,
			NeedsInlineCLA:    needsInlineCLA,
			Base:              baseRes,
		}
	}, krtopts.ToOptions("BaseEnvoyClusters")...)

	// Per-client deltas: only emitted for (ucc, backend) pairs that genuinely
	// vary — at least one PerClientClusterOverlay returned non-nil, or the
	// cluster requires a UCC-dependent inline CLA. Most pairs emit nothing,
	// which is what keeps the collection sparse.
	//
	// Driven off finalBackends so backend metadata-only updates (for example
	// Service labels consumed by an overlay) recompute deltas even when the
	// shared base cluster remains equal. The already-computed base is fetched
	// and reused, so UCC churn still does not re-translate base clusters.
	deltas := krt.NewCollection(finalBackends, func(kctx krt.HandlerContext, backendObj *ir.BackendObjectIR) *backendClusterDeltaSet {
		if backendObj == nil {
			return nil
		}
		// Base rows are keyed by cluster name, which translation always derives
		// from the backend's memoized ClusterName (blackhole included).
		baseObj := krt.FetchOne(kctx, base, krt.FilterKey(backendObj.ClusterName()))
		if baseObj == nil {
			return nil
		}
		b := *baseObj
		// FetchOne can briefly return the prior base generation. Do not mark the
		// new backend input resolved against that stale base.
		if b.Fingerprint.Input != fingerprintBackendInput(backendObj) {
			return nil
		}
		clients := krt.Fetch(kctx, uccs)
		set := &backendClusterDeltaSet{
			Name:               b.Name,
			BaseFingerprint:    b.Fingerprint,
			ClientsFingerprint: fingerprintClients(clients),
		}
		if b.Error != nil || b.Base == nil {
			// Errored base: every UCC sees the same blackhole, no per-client
			// variation possible. The empty set explicitly records resolution.
			return set
		}
		// Intern identical per-client clusters across UCCs. Inline-CLA backends
		// materialize a delta for every UCC, but UCCs that share the relevant
		// inputs (e.g. the same locality) produce byte-identical clusters; sharing
		// one proto instead of N clones cuts allocations to O(distinct). The protos
		// are read-only on the consumer side, so aliasing is safe.
		internByVersion := map[uint64]sharedproto.Shared[*envoyclusterv3.Cluster]{}
		for _, ucc := range clients {
			perClientBase := *b.Base
			perClientBase.Cluster = b.Cluster.Clone()
			perClient, err := translator.ApplyPerClient(kctx, ctx, ucc, backendObj, &perClientBase)
			if err != nil {
				// Emit a delta entry that carries the error so the snapshot
				// tracks this cluster as errored for THIS UCC only. Falling
				// back to the (valid) base would defeat strict-mode validation;
				// the user opted in to having broken configs surface as errors
				// rather than NACKs at the Envoy data plane.
				logger.Error("failed to apply per-client overlay",
					"backend", b.Name, "ucc", ucc.ResourceName(), "error", err)
				name := b.Name
				if perClient != nil {
					name = perClient.GetName()
				}
				if set.Deltas == nil {
					set.Deltas = make(map[string]uccClusterDelta)
				}
				set.Deltas[ucc.ResourceName()] = uccClusterDelta{
					Client:          ucc,
					Name:            name,
					BaseFingerprint: b.Fingerprint,
					// Hash 0: errored rows are never published, so they opt
					// out of tripwire verification.
					Cluster:        sharedproto.WrapPrehashed(perClient, 0),
					ClusterVersion: utils.HashString(err.Error()),
					Error:          err,
				}
				continue
			}
			if perClient == nil {
				// No per-client variation. Snapshot will reference the shared
				// base cluster instead.
				continue
			}
			clusterVersion := utils.HashProto(perClient)
			shared, ok := internByVersion[clusterVersion]
			if !ok {
				// clusterVersion IS the content hash, so tripwire capture is free.
				shared = sharedproto.WrapPrehashed(perClient, clusterVersion)
				internByVersion[clusterVersion] = shared
			}
			if set.Deltas == nil {
				set.Deltas = make(map[string]uccClusterDelta)
			}
			set.Deltas[ucc.ResourceName()] = uccClusterDelta{
				Client:          ucc,
				Name:            perClient.GetName(),
				BaseFingerprint: b.Fingerprint,
				Cluster:         shared,
				ClusterVersion:  clusterVersion,
			}
		}
		return set
	}, krtopts.ToOptions("PerClientEnvoyClusterDeltas")...)

	return PerClientEnvoyClusters{
		base:    base,
		deltas:  deltas,
		clients: uccs,
	}
}
