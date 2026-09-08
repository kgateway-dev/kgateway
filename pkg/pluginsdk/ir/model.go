package ir

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"maps"
	"slices"
	"strings"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

const KeyDelimiter = "~"

type PodLocality struct {
	Region  string
	Zone    string
	Subzone string
}

func (c PodLocality) String() string {
	return fmt.Sprintf("%s/%s/%s", c.Region, c.Zone, c.Subzone)
}

type UniquelyConnectedClient struct {
	Role      string
	Labels    map[string]string
	Locality  PodLocality
	Namespace string

	// KnowsLocalCluster reports whether this client's own EDS subscription has
	// named its expected LocalClusterName() resource at least once. Old Envoys
	// have no matching static cluster in their bootstrap and can never name it,
	// so this must default to false and only ever be set by observing a real
	// request (see pkg/krtcollections/uniqueclients.go) — never assume support.
	// Without this gate, go-control-plane's ADS "superset" check
	// (github.com/envoyproxy/go-control-plane pkg/cache/v3/simple.go) withholds
	// the *entire* EDS response to a client that doesn't ask for every resource
	// in the snapshot, breaking all endpoint updates for that client, not just
	// the local cluster (see https://github.com/kgateway-dev/kgateway/issues/14471).
	KnowsLocalCluster bool

	// modified role that includes the namespace and the hash of the labels.
	// we set the client's role to this value in the node metadata. so the snapshot key in the cache
	// should also be set to this value.
	resourceName string
}

func (c UniquelyConnectedClient) ResourceName() string {
	return c.resourceName
}

var _ krt.Equaler[UniquelyConnectedClient] = new(UniquelyConnectedClient)

func (c UniquelyConnectedClient) Equals(k UniquelyConnectedClient) bool {
	return c.Role == k.Role && c.Namespace == k.Namespace && c.Locality == k.Locality &&
		c.KnowsLocalCluster == k.KnowsLocalCluster && maps.Equal(c.Labels, k.Labels) && c.resourceName == k.resourceName
}

// GatewayNameFromLabels returns the Gateway name recorded on a pod or client,
// preferring the full (untruncated) name annotation over the possibly-hashed label.
func GatewayNameFromLabels(labels map[string]string) string {
	if labels == nil {
		return ""
	}
	if gatewayName := labels[wellknown.GatewayNameAnnotation]; gatewayName != "" {
		return gatewayName
	}
	return labels[wellknown.GatewayNameLabel]
}

// LocalClusterName returns the name of the per-gateway "local cluster" EDS resource that
// kgateway programs for native zone-aware routing. It is the single source of truth for this
// name and must stay in sync with the bootstrap config produced by the Helm template
// (see kgateway.gateway.fullname in pkg/kgateway/helm/envoy/templates/_helpers.tpl).
func LocalClusterName(gatewayName, gatewayNamespace string) string {
	return fmt.Sprintf("%s.%s", kubeutils.SafeGatewayLabelValue(gatewayName), gatewayNamespace)
}

// LocalClusterInfo derives this client's expected local-cluster resource name (see
// LocalClusterName), along with the gateway name/namespace it was derived from. Returns ""
// for clusterName if this client isn't associated with a single gateway.
func (c UniquelyConnectedClient) LocalClusterInfo() (clusterName, gatewayName, gatewayNamespace string) {
	gatewayNamespace = c.Namespace
	gatewayName = GatewayNameFromLabels(c.Labels)

	roleParts := strings.Split(c.Role, KeyDelimiter)
	if len(roleParts) == 3 {
		if gatewayNamespace == "" {
			gatewayNamespace = roleParts[1]
		}
		if gatewayName == "" {
			gatewayName = roleParts[2]
		}
	}

	if gatewayName == "" || gatewayNamespace == "" {
		return "", gatewayName, gatewayNamespace
	}
	return LocalClusterName(gatewayName, gatewayNamespace), gatewayName, gatewayNamespace
}

// note: if "ns" is empty, we assume the user doesn't want to use pod locality info, so we won't modify the role.
func NewUniquelyConnectedClient(roleFromEnvoy string, ns string, labels map[string]string, locality PodLocality) UniquelyConnectedClient {
	resourceName := roleFromEnvoy
	if ns != "" {
		snapshotKey := labeledRole(resourceName, labels)
		resourceName = fmt.Sprintf("%s%s%s", snapshotKey, KeyDelimiter, ns)
	}
	return UniquelyConnectedClient{
		Role:         roleFromEnvoy,
		Namespace:    ns,
		Locality:     locality,
		Labels:       labels,
		resourceName: resourceName,
	}
}

func labeledRole(role string, labels map[string]string) string {
	return fmt.Sprintf("%s%s%d", role, KeyDelimiter, utils.HashLabels(labels))
}

type EndpointMetadata struct {
	Labels map[string]string
}
type EndpointWithMd struct {
	*envoyendpointv3.LbEndpoint
	EndpointMd EndpointMetadata

	// endpointEqualityHash is the contribution hashEndpoints computed when this
	// endpoint was added to its locality. It is derived state, not another hash
	// input. Endpoint replacement builders reuse it for structurally shared
	// endpoints instead of marshaling the same proto once per client.
	//
	// Zero means "no cached contribution". Clone clears it so a copy taken in
	// order to be modified cannot version different content identically, and
	// ReuseEndpoint recomputes when it sees zero.
	endpointEqualityHash uint64
}

// Clone returns a transitively isolated copy of the endpoint that the caller
// owns: the LbEndpoint proto and the metadata labels are both deep copied, so
// neither the source nor any other holder of it observes later writes.
//
// The cached contribution hash is deliberately dropped. A clone exists to be
// modified, and carrying the original's hash into ReuseEndpoint would publish
// changed endpoint content under the unchanged version Envoy already has.
func (e EndpointWithMd) Clone() EndpointWithMd {
	out := e
	out.endpointEqualityHash = 0
	if e.LbEndpoint != nil {
		out.LbEndpoint = proto.Clone(e.LbEndpoint).(*envoyendpointv3.LbEndpoint)
	}
	out.EndpointMd.Labels = maps.Clone(e.EndpointMd.Labels)
	return out
}

type LocalityLbMap map[PodLocality][]EndpointWithMd

// MarshalJSON implements json.Marshaler. for krt.DebugHandler
func (l LocalityLbMap) MarshalJSON() ([]byte, error) {
	out := map[string][]EndpointWithMd{}
	for locality, eps := range l {
		out[locality.String()] = eps
	}
	return json.Marshal(out)
}

var _ json.Marshaler = LocalityLbMap{}

type EndpointsForBackend struct {
	// preserve the labels from the original backend object
	// for use in endpoints plugins
	// +krtEqualsTodo include backend labels in equality or confirm omission
	BackendLabels map[string]string

	// AttachedPolicies carries the policy attachment view already resolved for
	// the backend. LbEpsEqualityHash includes backend policy versioning, so this
	// field does not need to participate in equality directly.
	// +noKrtEquals
	AttachedPolicies AttachedPolicies

	// +krtEqualsTodo compare load-balanced endpoint map
	LbEps                LocalityLbMap
	ClusterName          string
	UpstreamResourceName string
	Port                 uint32
	Hostname             string
	// Inherited from the backend object
	TrafficDistribution wellknown.TrafficDistribution

	LbEpsEqualityHash uint64
	upstreamHash      uint64
	epsEqualityHash   uint64
	// endpointCount is the number of endpoints across all localities. The
	// equality hash needs the empty/non-empty distinction, and it cannot be
	// derived from epsEqualityHash: a non-empty set can xor to zero. Maintaining
	// it here keeps every writer — Add, ReuseEndpointsFrom, AdoptEndpointsFrom —
	// answering that question the same way.
	endpointCount        int
	foldedVersionHash    uint64
	hasFoldedVersionHash bool
}

func NewEndpointsForBackend(us BackendObjectIR) *EndpointsForBackend {
	labels := map[string]string{}
	if us.Obj != nil && us.Obj.GetLabels() != nil {
		labels = us.Obj.GetLabels()
	}

	// Start with the backend identity we expose to Envoy. We still include both
	// ResourceName and ClusterName here so Gateway-scoped backend clones that
	// share the same Service and endpoints, but differ in client certificate
	// identity, do not collapse to the same endpoint hash.
	// note: we no longer need to add the upstream body hash to the clustername, as we applied `use_eds_cache_for_ads`
	// to mitigate https://github.com/envoyproxy/envoy/issues/13070 / https://github.com/envoyproxy/envoy/issues/13009

	h := fnv.New64a()
	objSrc := us.GetObjectSource()
	h.Write([]byte(us.ResourceName()))
	h.Write([]byte{0})
	h.Write([]byte(us.ClusterName()))
	h.Write([]byte{0})
	h.Write([]byte(objSrc.Group))
	h.Write([]byte{0})
	h.Write([]byte(objSrc.Kind))
	h.Write([]byte{0})
	h.Write([]byte(objSrc.Name))
	h.Write([]byte{0})
	h.Write([]byte(objSrc.Namespace))
	for k, v := range labels {
		h.Write([]byte{0})
		h.Write([]byte(k + "=" + v))
	}
	h.Write([]byte{0})
	h.Write([]byte{byte(us.TrafficDistribution)})
	upstreamHash := h.Sum64()

	return &EndpointsForBackend{
		BackendLabels:        labels,
		AttachedPolicies:     us.AttachedPolicies,
		LbEps:                make(map[PodLocality][]EndpointWithMd),
		ClusterName:          us.ClusterName(),
		UpstreamResourceName: us.ResourceName(),
		Port:                 uint32(us.GetPort()), //nolint:gosec // G115: upstream port is always valid port range
		Hostname:             us.CanonicalHostname,
		LbEpsEqualityHash:    upstreamHash,
		upstreamHash:         upstreamHash,
		TrafficDistribution:  us.TrafficDistribution,
	}
}

// EmptyCopy creates a fresh EndpointsForBackend with no endpoints
// for the same backend.
func (e EndpointsForBackend) EmptyCopy() EndpointsForBackend {
	out := EndpointsForBackend{
		BackendLabels:        e.BackendLabels,
		AttachedPolicies:     e.AttachedPolicies,
		LbEps:                make(map[PodLocality][]EndpointWithMd),
		ClusterName:          e.ClusterName,
		UpstreamResourceName: e.UpstreamResourceName,
		Port:                 e.Port,
		Hostname:             e.Hostname,
		upstreamHash:         e.upstreamHash,
		TrafficDistribution:  e.TrafficDistribution,
		foldedVersionHash:    e.foldedVersionHash,
		hasFoldedVersionHash: e.hasFoldedVersionHash,
	}
	out.refreshLbEpsEqualityHash()
	return out
}

// FoldVersion mixes an extra input into this row's equality hash. Use it for
// state that changes what these endpoints translate into without changing the
// endpoints themselves — newFinalBackendEndpoints folds in the attached-policy
// hash for exactly that reason.
//
// Folded versions are tracked separately from endpoint content. EmptyCopy
// preserves them, and Add recomputes the final equality hash from both parts,
// so endpoint edits cannot silently erase an earlier contribution.
func (e *EndpointsForBackend) FoldVersion(extra uint64) {
	if e.hasFoldedVersionHash {
		e.foldedVersionHash = hash(e.foldedVersionHash, extra)
	} else {
		e.foldedVersionHash = extra
		e.hasFoldedVersionHash = true
	}
	e.refreshLbEpsEqualityHash()
}

func hashEndpoints(l PodLocality, emd EndpointWithMd) uint64 {
	hasher := fnv.New64a()
	hasher.Write([]byte(l.Region))
	hasher.Write([]byte(l.Zone))
	hasher.Write([]byte(l.Subzone))

	utils.HashUint64(hasher, utils.HashLabels(emd.EndpointMd.Labels))
	utils.HashProtoWithHasher(hasher, emd.LbEndpoint)
	return hasher.Sum64()
}

func hash(a, b uint64) uint64 {
	hasher := fnv.New64a()
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[:8], a)
	binary.LittleEndian.PutUint64(buf[8:], b)
	hasher.Write(buf[:])
	return hasher.Sum64()
}

func (e *EndpointsForBackend) Add(l PodLocality, emd EndpointWithMd) {
	e.addWithEndpointHash(l, emd, hashEndpoints(l, emd))
}

// ReuseEndpoint adds an immutable endpoint in the same locality from which it
// was read, reusing the contribution hash computed by Add. Callers that change
// either the endpoint or its locality must use Add so the contribution is
// recomputed.
//
// A zero cached hash means the endpoint was never hashed by Add, or was cloned
// in order to be modified. Rather than trust it, recompute: versioning changed
// content under an unchanged hash is the one failure this cache must not cause.
func (e *EndpointsForBackend) ReuseEndpoint(l PodLocality, emd EndpointWithMd) {
	endpointHash := emd.endpointEqualityHash
	if endpointHash == 0 {
		endpointHash = hashEndpoints(l, emd)
	}
	e.addWithEndpointHash(l, emd, endpointHash)
}

func (e *EndpointsForBackend) addWithEndpointHash(l PodLocality, emd EndpointWithMd, endpointHash uint64) {
	// xor it as we dont care about order - if we have the same endpoints in the same locality
	// we are good.
	e.epsEqualityHash ^= endpointHash
	emd.endpointEqualityHash = endpointHash
	e.LbEps[l] = append(e.LbEps[l], emd)
	e.endpointCount++
	e.refreshLbEpsEqualityHash()
}

func (e *EndpointsForBackend) refreshLbEpsEqualityHash() {
	endpointHash := e.upstreamHash
	if e.endpointCount > 0 {
		// We can't xor the endpoint hash with the upstream hash, because
		// upstreams with different names and similar endpoints will cancel out,
		// so endpoint changes won't result in different equality hashes.
		endpointHash = hash(e.epsEqualityHash, e.upstreamHash)
	}
	if e.hasFoldedVersionHash {
		endpointHash = hash(endpointHash, e.foldedVersionHash)
	}
	e.LbEpsEqualityHash = endpointHash
}

// ReuseEndpointsFrom copies the endpoint entries and their precomputed
// equality hash from base, avoiding a per-endpoint proto re-hash (which
// marshals every LbEndpoint). The equality hashes are identical to what a full
// re-Add of every endpoint would produce: epsEqualityHash only depends on the
// endpoint set (shared with base), and the final hash mixes in this copy's own
// upstreamHash for backend identity.
// The per-locality slices are cloned to avoid append aliasing with base. The
// endpoint map is rebuilt from scratch, so any stale localities previously
// present in e are dropped.
func (e *EndpointsForBackend) ReuseEndpointsFrom(base *EndpointsForBackend) {
	e.epsEqualityHash = base.epsEqualityHash
	e.endpointCount = base.endpointCount
	e.LbEps = make(LocalityLbMap, len(base.LbEps))
	for l, eps := range base.LbEps {
		e.LbEps[l] = slices.Clone(eps)
	}
	e.refreshLbEpsEqualityHash()
}

// AdoptEndpointsFrom moves src's endpoint set and its content hash onto e,
// leaving e's own backend identity and folded version in place. Unlike
// ReuseEndpointsFrom it takes the per-locality slices as they are: the caller
// must own src and must not use it afterwards.
//
// This is what installing a replacement endpoint set is made of. Overwriting e
// with src wholesale would also reinstate whatever identity fields src happened
// to be seeded with, silently reverting anything written to e while the
// replacement was being built.
func (e *EndpointsForBackend) AdoptEndpointsFrom(src *EndpointsForBackend) {
	e.LbEps = src.LbEps
	e.epsEqualityHash = src.epsEqualityHash
	e.endpointCount = src.endpointCount
	e.refreshLbEpsEqualityHash()
}

func (c EndpointsForBackend) ResourceName() string {
	return c.UpstreamResourceName
}

func (c EndpointsForBackend) Equals(in EndpointsForBackend) bool {
	return c.UpstreamResourceName == in.UpstreamResourceName && c.ClusterName == in.ClusterName && c.Port == in.Port && c.LbEpsEqualityHash == in.LbEpsEqualityHash && c.Hostname == in.Hostname && c.TrafficDistribution == in.TrafficDistribution && c.upstreamHash == in.upstreamHash && c.epsEqualityHash == in.epsEqualityHash && c.endpointCount == in.endpointCount && c.foldedVersionHash == in.foldedVersionHash && c.hasFoldedVersionHash == in.hasFoldedVersionHash
}
