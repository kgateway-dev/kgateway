package ir

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"maps"

	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
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

type UniqlyConnectedClient struct {
	Role      string
	Labels    map[string]string
	Locality  PodLocality
	Namespace string

	// modified role that includes the namespace and the hash of the labels.
	// we set the client's role to this value in the node metadata. so the snapshot key in the cache
	// should also be set to this value.
	resourceName string
}

func (c UniqlyConnectedClient) ResourceName() string {
	return c.resourceName
}

var _ krt.Equaler[UniqlyConnectedClient] = new(UniqlyConnectedClient)

func (c UniqlyConnectedClient) Equals(k UniqlyConnectedClient) bool {
	return c.Role == k.Role && c.Namespace == k.Namespace && c.Locality == k.Locality && maps.Equal(c.Labels, k.Labels)
}

// note: if "ns" is empty, we assume the user doesn't want to use pod locality info, so we won't modify the role.
func NewUniqlyConnectedClient(roleFromEnvoy string, ns string, labels map[string]string, locality PodLocality) UniqlyConnectedClient {
	resourceName := roleFromEnvoy
	if ns != "" {
		snapshotKey := labeledRole(resourceName, labels)
		resourceName = fmt.Sprintf("%s%s%s", snapshotKey, KeyDelimiter, ns)
	}
	return UniqlyConnectedClient{
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
	*envoy_config_endpoint_v3.LbEndpoint
	EndpointMd EndpointMetadata
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
	BackendLabels map[string]string

	LbEps LocalityLbMap
	// Note - in theory, cluster name should be a function of the UpstreamResourceName.
	// But due to an upstream envoy bug, the cluster name also includes the upstream hash.
	ClusterName          string
	UpstreamResourceName string
	Port                 uint32
	Hostname             string

	LbEpsEqualityHash uint64
	upstreamHash      uint64
	epsEqualityHash   uint64
}

func NewEndpointsForBackend(us BackendObjectIR) *EndpointsForBackend {
	labels := map[string]string{}
	if us.Obj != nil && us.Obj.GetLabels() != nil {
		labels = us.Obj.GetLabels()
	}

	// start with a hash of the cluster name. technically we dont need it for krt, as we can compare the upstream name. but it helps later
	// to compute the hash we present envoy with.
	// note: we no longer need to add the upstream body hash to the clustername, as we applied `use_eds_cache_for_ads`
	// to mitigate https://github.com/envoyproxy/envoy/issues/13070 / https://github.com/envoyproxy/envoy/issues/13009

	h := fnv.New64a()
	h.Write([]byte(us.Group))
	h.Write([]byte{0})
	h.Write([]byte(us.Kind))
	h.Write([]byte{0})
	h.Write([]byte(us.Name))
	h.Write([]byte{0})
	h.Write([]byte(us.Namespace))
	for k, v := range labels {
		h.Write([]byte{0})
		h.Write([]byte(k + "=" + v))
	}
	upstreamHash := h.Sum64()

	return &EndpointsForBackend{
		BackendLabels:        labels,
		LbEps:                make(map[PodLocality][]EndpointWithMd),
		ClusterName:          us.ClusterName(),
		UpstreamResourceName: us.ResourceName(),
		Port:                 uint32(us.Port),
		Hostname:             us.CanonicalHostname,
		LbEpsEqualityHash:    upstreamHash,
		upstreamHash:         upstreamHash,
	}
}

// EmptyCopy creates a fresh EndpointsForBackend with no endpoints
// for the same backend.
func (e EndpointsForBackend) EmptyCopy() EndpointsForBackend {
	return EndpointsForBackend{
		BackendLabels:        e.BackendLabels,
		LbEps:                make(map[PodLocality][]EndpointWithMd),
		ClusterName:          e.ClusterName,
		UpstreamResourceName: e.UpstreamResourceName,
		Port:                 e.Port,
		Hostname:             e.Hostname,
		LbEpsEqualityHash:    e.upstreamHash,
		upstreamHash:         e.upstreamHash,
	}
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
	// xor it as we dont care about order - if we have the same endpoints in the same locality
	// we are good.
	e.epsEqualityHash ^= hashEndpoints(l, emd)
	// we can't xor the endpoint hash with the upstream hash, because upstreams with
	// different names and similar endpoints will cancel out, so endpoint changes
	// won't result in different equality hashes.
	e.LbEpsEqualityHash = hash(e.epsEqualityHash, e.upstreamHash)
	e.LbEps[l] = append(e.LbEps[l], emd)
}

func (c EndpointsForBackend) ResourceName() string {
	return c.UpstreamResourceName
}

func (c EndpointsForBackend) Equals(in EndpointsForBackend) bool {
	return c.UpstreamResourceName == in.UpstreamResourceName && c.ClusterName == in.ClusterName && c.Port == in.Port && c.LbEpsEqualityHash == in.LbEpsEqualityHash && c.Hostname == in.Hostname
}
