package proxy_syncer

import (
	"cmp"
	"context"
	"hash/fnv"
	"maps"
	"slices"
	"sync"

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
	// Fingerprint fences per-client deltas to the base cluster content they were
	// cloned from. ClusterVersion includes the proto and any inline endpoint or
	// policy inputs consumed later by ApplyPerClient.
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

// baseClusterFingerprint identifies the base cluster content a per-client delta
// was cloned from. A delta set carrying a different ClusterVersion than the base
// it is being merged with must not be published.
type baseClusterFingerprint struct {
	ClusterVersion uint64
}

func fingerprintBase(clusterVersion uint64) baseClusterFingerprint {
	return baseClusterFingerprint{ClusterVersion: clusterVersion}
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
// backend's sparse delta set was evaluated. It drives KRT equality for the
// retained snapshot; when a sparse delta is absent, read-side readiness uses
// that snapshot's exact per-client membership rather than comparing this
// fleet-wide value.
type clientSetFingerprint uint64

// fingerprintClients hashes the client set order-independently, over exactly the
// fields an overlay can branch on (role, namespace, locality, labels, and local
// cluster capability). Two calls agree if and only if every client an overlay
// could distinguish is unchanged.
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
		if client.KnowsLocalCluster {
			utils.HashUint64(hasher, 1)
		} else {
			utils.HashUint64(hasher, 0)
		}
	}
	return clientSetFingerprint(hasher.Sum64())
}

// clientInputSnapshot is one immutable view of the UCC collection. Every
// backend delta set computed from this view retains a shallow copy of the
// snapshot, so the Clients slice and byName map are shared across backends.
// This lets a reader prove that one specific client was evaluated without
// requiring every backend to agree on the latest fleet-wide generation.
type clientInputSnapshot struct {
	Fingerprint clientSetFingerprint
	Clients     []ir.UniquelyConnectedClient
	// byName is derived from Clients and is immutable after construction.
	// +noKrtEquals
	byName map[string]ir.UniquelyConnectedClient
}

func newClientInputSnapshot(clients []ir.UniquelyConnectedClient) clientInputSnapshot {
	return newClientInputSnapshotWithFingerprint(clients, fingerprintClients(clients))
}

func newClientInputSnapshotWithFingerprint(
	clients []ir.UniquelyConnectedClient,
	fingerprint clientSetFingerprint,
) clientInputSnapshot {
	ordered := slices.Clone(clients)
	for i := range ordered {
		// UCCs are values except for Labels. Clone that map so a plugin cannot
		// mutate the resolution proof retained by every backend set.
		ordered[i].Labels = maps.Clone(ordered[i].Labels)
	}
	slices.SortFunc(ordered, func(a, b ir.UniquelyConnectedClient) int {
		return cmp.Compare(a.ResourceName(), b.ResourceName())
	})
	byName := make(map[string]ir.UniquelyConnectedClient, len(ordered))
	for _, client := range ordered {
		proof := client
		proof.Labels = maps.Clone(client.Labels)
		byName[client.ResourceName()] = proof
	}
	return clientInputSnapshot{
		Fingerprint: fingerprint,
		Clients:     ordered,
		byName:      byName,
	}
}

func (s clientInputSnapshot) matches(clients []ir.UniquelyConnectedClient) bool {
	if len(s.Clients) != len(clients) {
		return false
	}
	for _, client := range clients {
		evaluated, ok := s.byName[client.ResourceName()]
		if !ok || !evaluated.Equals(client) {
			return false
		}
	}
	return true
}

// ContainsCurrent reports whether this snapshot evaluated the exact current
// version of client. ResourceName alone is insufficient because fields that do
// not participate in the key, notably KnowsLocalCluster, affect overlays.
func (s clientInputSnapshot) ContainsCurrent(client ir.UniquelyConnectedClient) bool {
	evaluated, ok := s.byName[client.ResourceName()]
	return ok && evaluated.Equals(client)
}

// clientInputSnapshotInterner shares one immutable snapshot across backend
// transforms without inserting another KRT collection between UCC events and
// delta recomputation. It retains only the most recently requested snapshot;
// older snapshots stay alive solely while backend delta sets still reference
// their slices and maps during convergence.
type clientInputSnapshotInterner struct {
	mu     sync.Mutex
	latest *clientInputSnapshot
}

func (i *clientInputSnapshotInterner) intern(clients []ir.UniquelyConnectedClient) clientInputSnapshot {
	fingerprint := fingerprintClients(clients)
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.latest != nil && i.latest.Fingerprint == fingerprint && i.latest.matches(clients) {
		return *i.latest
	}
	snapshot := newClientInputSnapshotWithFingerprint(clients, fingerprint)
	i.latest = &snapshot
	return snapshot
}

// backendClusterDeltaSet is the atomic sparse overlay result for one backend.
// A row exists even when Deltas is empty. ResolvedClients disambiguates "this
// exact client was evaluated and needs no overlay" from "this client was not
// evaluated yet" without making unrelated fleet churn a readiness barrier.
type backendClusterDeltaSet struct {
	Name            string
	BaseFingerprint baseClusterFingerprint
	// ClientsFingerprint participates in KRT equality so a new resolution
	// snapshot replaces the stored row. It is not a read-side readiness gate:
	// readers consult only their own entry in ResolvedClients, and only when no
	// materialized delta already proves that exact client was evaluated.
	ClientsFingerprint clientSetFingerprint
	// ResolvedClients is the exact immutable UCC snapshot consumed while Deltas
	// was built. Its content equality is represented by ClientsFingerprint.
	// +noKrtEquals
	ResolvedClients clientInputSnapshot
	// Deltas contains only clients whose cluster genuinely differs from base.
	// +noKrtEquals
	Deltas map[string]uccClusterDelta
}

// clientBackendDeltaView is the portion of one backend's delta set relevant to
// one requesting client. FetchClustersForClient uses it with krt.PartialFetch so
// changes concerning other clients do not retrigger this client's CDS assembly.
type clientBackendDeltaView struct {
	Name            string
	BaseFingerprint baseClusterFingerprint
	Resolved        bool
	HasDelta        bool
	Delta           uccClusterDelta
}

func (d clientBackendDeltaView) Equals(in clientBackendDeltaView) bool {
	if d.Name != in.Name ||
		d.BaseFingerprint != in.BaseFingerprint ||
		d.HasDelta != in.HasDelta {
		return false
	}
	if d.HasDelta {
		// A materialized delta carries its exact UCC input, so fleet-resolution
		// changes are irrelevant while that delta remains current.
		return d.Delta.Equals(in.Delta)
	}
	// Resolution is needed only to interpret sparse absence as an affirmative
	// "no overlay applies" result.
	return d.Resolved == in.Resolved
}

func (d backendClusterDeltaSet) forClient(client ir.UniquelyConnectedClient) clientBackendDeltaView {
	delta, hasDelta := d.Deltas[client.ResourceName()]
	return clientBackendDeltaView{
		Name:            d.Name,
		BaseFingerprint: d.BaseFingerprint,
		Resolved:        d.ResolvedClients.ContainsCurrent(client),
		HasDelta:        hasDelta,
		Delta:           delta,
	}
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

// errString renders an error for comparison inside an Equals method, where a nil
// error and an empty message must compare equal.
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

// PerClientEnvoyClusters is the cluster half of per-client xDS, stored as a base
// plus a sparse overlay rather than one row per (client, backend) pair:
//
//   - base holds the UCC-invariant translation, one row per backend, whose Cluster
//     proto is shared read-only by every client that targets it.
//   - deltas holds one row per backend recording which clients — usually none —
//     genuinely need a different cluster, and what it is.
//   - clients is retained so a read can verify that the requesting UCC is still
//     connected without depending on unrelated clients.
//
// Consumers never read the fields directly: FetchClustersForClient merges base and
// delta into the view a snapshot needs, and StatusClusters projects the errors out
// for status. Construct with [NewPerClientEnvoyClusters].
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
// delta set was evaluated against the current base and this exact client. A
// mismatch returns no rows, causing snapshot assembly to retain this client's
// last coherent xDS snapshot while the dependent KRT transforms catch up.
//
// The *Cluster protos in the returned slice are shared with other UCCs (base)
// or unique to this UCC (delta); callers MUST NOT mutate them.
func (iu *PerClientEnvoyClusters) FetchClustersForClient(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) []uccWithCluster {
	var bases []baseEnvoyCluster
	if iu.base != nil {
		bases = krt.Fetch(kctx, iu.base)
	}
	var deltaViews []clientBackendDeltaView
	if iu.deltas != nil {
		deltaViews = krt.PartialFetch(kctx, iu.deltas,
			func(set backendClusterDeltaSet) clientBackendDeltaView {
				return set.forClient(ucc)
			},
			clientBackendDeltaView.Equals,
		)
	}

	deltaViewByName := make(map[string]clientBackendDeltaView, len(deltaViews))
	for _, view := range deltaViews {
		deltaViewByName[view.Name] = view
	}

	if iu.clients != nil {
		// Narrow this dependency to the requesting UCC. Fleet churn must not
		// retrigger every established client's cluster transform.
		current := krt.FetchOne(kctx, iu.clients, krt.FilterKey(ucc.ResourceName()))
		if current == nil || !current.Equals(ucc) {
			return nil
		}
	}

	// Validate the entire generation before exposing any row. Returning a
	// partial base/delta mix would let snapshotPerClient publish an incoherent
	// CDS payload while collections converge.
	for _, b := range bases {
		view, ok := deltaViewByName[b.Name]
		if !ok || view.BaseFingerprint != b.Fingerprint {
			return nil
		}
		if view.HasDelta {
			if !view.Delta.Client.Equals(ucc) {
				return nil
			}
		} else {
			if !view.Resolved {
				return nil
			}
			if b.NeedsInlineCLA {
				// Inline-CLA bases must always materialize a per-client delta.
				return nil
			}
		}
	}

	out := make([]uccWithCluster, 0, len(bases))
	for _, b := range bases {
		view := deltaViewByName[b.Name]
		if view.HasDelta {
			d := view.Delta
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
// resolved-client snapshot is deliberately not re-checked here. A delta set whose
// inputs moved is recomputed anyway (ClientsFingerprint participates in its Equals),
// so at worst a departed client's error lingers for one propagation.
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
			if d.Error == nil {
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

// NewPerClientEnvoyClusters builds the base and delta collections that back
// [PerClientEnvoyClusters], translating every backend in finalBackends into an
// Envoy cluster for every client in uccs.
//
// The work is split so that the expensive part does not scale with the client
// count: each backend is translated once into a shared base, and each connected
// client is then offered a cheap overlay on top of it. Only the (client, backend)
// pairs whose cluster genuinely differs — a matching destination rule, a waypoint
// redirect, an inline CLA, a per-client validation failure — materialize a delta.
// For a fleet where few backends vary per client, storage and translation cost stay
// close to O(backends) instead of O(backends * clients).
//
// The two collections are versioned against each other by fingerprint, and each
// delta set retains the immutable client snapshot it consumed. A consumer can
// therefore verify the base generation and the requesting client independently.
// See FetchClustersForClient for how that is enforced.
func NewPerClientEnvoyClusters(
	ctx context.Context,
	krtopts krtutil.KrtOptions,
	translator *irtranslator.BackendTranslator,
	finalBackends krt.Collection[*ir.BackendObjectIR],
	uccs krt.Collection[ir.UniquelyConnectedClient],
) PerClientEnvoyClusters {
	// Share immutable UCC snapshots across backend transforms without adding an
	// extra KRT propagation hop between a UCC event and delta recomputation.
	clientInputs := &clientInputSnapshotInterner{}

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
			Fingerprint:       fingerprintBase(clusterVersion),
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
		clientSnapshot := clientInputs.intern(krt.Fetch(kctx, uccs))
		set := &backendClusterDeltaSet{
			Name:               b.Name,
			BaseFingerprint:    b.Fingerprint,
			ClientsFingerprint: clientSnapshot.Fingerprint,
			ResolvedClients:    clientSnapshot,
		}
		if b.Error != nil || b.Base == nil {
			// Errored base: every UCC sees the same blackhole, no per-client
			// variation possible. The empty set explicitly records resolution.
			return set
		}
		// One clone for the whole client loop, not one per client. ApplyPerClient
		// only reads this proto — it clones internally before letting an overlay
		// touch it — so the copy exists solely to unseal the shared base, which is
		// a per-backend concern. Cloning per client would restore the O(backends *
		// clients) deep copy that the base/overlay split exists to remove, and pay
		// it on the dominant path where ApplyPerClient returns nil untouched.
		perClientBase := *b.Base
		perClientBase.Cluster = b.Cluster.Clone()
		for _, ucc := range clientSnapshot.Clients {
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
					Client: ucc,
					Name:   name,
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
			if set.Deltas == nil {
				set.Deltas = make(map[string]uccClusterDelta)
			}
			set.Deltas[ucc.ResourceName()] = uccClusterDelta{
				Client:         ucc,
				Name:           perClient.GetName(),
				Cluster:        sharedproto.WrapPrehashed(perClient, clusterVersion),
				ClusterVersion: clusterVersion,
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
