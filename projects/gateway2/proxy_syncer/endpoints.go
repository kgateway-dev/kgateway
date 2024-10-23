package proxy_syncer

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gloo/constants"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type EndpointMetadata struct {
	Labels map[string]string
}

type EndpointsInputs struct {
	Upstreams      krt.Collection[UpstreamWrapper]
	Endpoints      krt.Collection[*corev1.Endpoints]
	Pods           krt.Collection[krtcollections.LocalityPod]
	EnableAutoMtls bool
	Services       krt.Collection[*corev1.Service]
}

func NewGlooK8sEndpointInputs(settings *v1.Settings, istioClient kube.Client, pods krt.Collection[krtcollections.LocalityPod], services krt.Collection[*corev1.Service], finalUpstreams krt.Collection[UpstreamWrapper]) EndpointsInputs {
	epClient := kclient.New[*corev1.Endpoints](istioClient)
	kubeEndpoints := krt.WrapClient(epClient, krt.WithName("Endpoints"))
	enableAutoMtls := settings.GetGloo().GetIstioOptions().GetEnableAutoMtls().GetValue()

	return EndpointsInputs{
		Upstreams:      finalUpstreams,
		Endpoints:      kubeEndpoints,
		Pods:           pods,
		EnableAutoMtls: enableAutoMtls,
		Services:       services,
	}
}

type EndpointWithMd struct {
	*envoy_config_endpoint_v3.LbEndpoint
	EndpointMd EndpointMetadata
}
type EndpointsForUpstream struct {
	LbEps       map[krtcollections.PodLocality][]EndpointWithMd
	clusterName string
	UpstreamRef types.NamespacedName

	lbEpsEqualityHash uint64
}

func NewEndpointsForUpstream(us UpstreamWrapper) *EndpointsForUpstream {
	clusterName := translator.UpstreamToClusterName(us.Inner.GetMetadata().Ref())
	// start with a hash of the cluster name. technically we dont need it for krt, as we can compare the upstream name. but it helps later
	// to compute the hash we present envoy with.
	h := fnv.New64()
	h.Write([]byte(clusterName))
	lbEpsEqualityHash := h.Sum64()

	// add the upstream hash to the clustername, so that if it changes the envoy cluster will become warm again.
	clusterName = getEndpointClusterName(clusterName, us.Inner)
	return &EndpointsForUpstream{
		LbEps:       make(map[krtcollections.PodLocality][]EndpointWithMd),
		clusterName: clusterName,
		UpstreamRef: types.NamespacedName{
			Namespace: us.Inner.GetMetadata().Namespace,
			Name:      us.Inner.GetMetadata().Name,
		},
		lbEpsEqualityHash: lbEpsEqualityHash,
	}
}

func (e *EndpointsForUpstream) Add(l krtcollections.PodLocality, emd EndpointWithMd) {
	hasher := fnv.New64()
	hasher.Write([]byte(l.Region))
	hasher.Write([]byte(l.Zone))
	hasher.Write([]byte(l.Subzone))

	addr := emd.GetEndpoint().GetAddress().GetSocketAddress().GetAddress()
	port := emd.GetEndpoint().GetAddress().GetSocketAddress().GetPortValue()
	hasher.Write([]byte(addr))
	hashUint64(hasher, uint64(port))
	hashUint64(hasher, hashLabels(emd.EndpointMd.Labels))
	hashUint64(hasher, HashMetadata(fnv.New64, emd.GetMetadata()))

	// xor it as we dont care about order - if we have the same endpoints in the same locality
	// we are good.
	e.lbEpsEqualityHash ^= hasher.Sum64()
	e.LbEps[l] = append(e.LbEps[l], emd)
}

func (c EndpointsForUpstream) ResourceName() string {
	return c.UpstreamRef.String()
}

func (c EndpointsForUpstream) Equals(in EndpointsForUpstream) bool {
	return c.UpstreamRef == in.UpstreamRef && c.lbEpsEqualityHash == in.lbEpsEqualityHash
}

func NewGlooK8sEndpoints(ctx context.Context, inputs EndpointsInputs) krt.Collection[EndpointsForUpstream] {
	return krt.NewCollection(inputs.Upstreams, TransformUpstreamsBuilder(ctx, inputs), krt.WithName("GlooK8sEndpoints"))
}

func TransformUpstreamsBuilder(ctx context.Context, inputs EndpointsInputs) func(kctx krt.HandlerContext, us UpstreamWrapper) *EndpointsForUpstream {
	augmentedPods := inputs.Pods
	kubeEndpoints := inputs.Endpoints
	enableAutoMtls := inputs.EnableAutoMtls
	services := inputs.Services

	logger := contextutils.LoggerFrom(ctx)

	return func(kctx krt.HandlerContext, us UpstreamWrapper) *EndpointsForUpstream {
		var warnsToLog []string
		defer func() {
			for _, warn := range warnsToLog {
				logger.Warn(warn)
			}
		}()
		logger := logger.With("upstream", us.Inner.GetMetadata().Ref().Key())

		logger.Debugf("building endpoints")

		kubeUpstream, ok := us.Inner.GetUpstreamType().(*v1.Upstream_Kube)
		// only care about kube upstreams
		if !ok {
			return nil
		}
		spec := kubeUpstream.Kube
		kubeServicePort, singlePortService := findPortForService(kctx, services, spec)
		if kubeServicePort == nil {
			logger.Debugf("findPortForService - not found. port: %d. service %s.%s", spec.GetServicePort(), spec.GetServiceName(), spec.GetServiceNamespace())
			return nil
		}

		maybeEps := krt.FetchOne(kctx, kubeEndpoints, krt.FilterObjectName(types.NamespacedName{
			Namespace: spec.GetServiceNamespace(),
			Name:      spec.GetServiceName(),
		}))
		if maybeEps == nil {
			warnsToLog = append(warnsToLog, fmt.Sprintf("endpoints not found for service %v", spec.GetServiceName()))
			return nil
		}
		eps := *maybeEps

		ret := NewEndpointsForUpstream(us)
		for _, subset := range eps.Subsets {
			port := findFirstPortInEndpointSubsets(subset, singlePortService, kubeServicePort)
			if port == 0 {
				warnsToLog = append(warnsToLog, fmt.Sprintf("port not found (%v) for service %v in endpoint %v", spec.GetServicePort(), spec.GetServiceName(), subset))
				continue
			}

			for _, addr := range subset.Addresses {
				var podName string
				podNamespace := eps.Namespace
				targetRef := addr.TargetRef
				if targetRef != nil {
					if targetRef.Kind == "Pod" {
						podName = targetRef.Name
						if targetRef.Namespace != "" {
							podNamespace = targetRef.Namespace
						}
					}
				}

				var podLabels map[string]string
				var augmentedLabels map[string]string
				var l krtcollections.PodLocality
				if podName != "" {
					maybePod := krt.FetchOne(kctx, augmentedPods, krt.FilterObjectName(types.NamespacedName{
						Namespace: podNamespace,
						Name:      podName,
					}))
					if maybePod != nil {
						l = maybePod.Locality
						podLabels = maybePod.AugmentedLabels
						augmentedLabels = maybePod.AugmentedLabels
					}
				}
				ep := createLbEndpoint(addr.IP, port, podLabels, enableAutoMtls)

				ret.Add(l, EndpointWithMd{
					LbEndpoint: ep,
					EndpointMd: EndpointMetadata{
						Labels: augmentedLabels,
					},
				})
			}
		}
		return ret
	}
}

func createLbEndpoint(address string, port uint32, podLabels map[string]string, enableAutoMtls bool) *envoy_config_endpoint_v3.LbEndpoint {
	// Don't get the metadata labels and filter metadata for the envoy load balancer based on the upstream, as this is not used
	// metadata := getLbMetadata(upstream, labels, "")
	// Get the metadata labels for the transport socket match if Istio auto mtls is enabled
	metadata := &envoy_config_core_v3.Metadata{
		FilterMetadata: map[string]*structpb.Struct{},
	}
	metadata = addIstioAutomtlsMetadata(metadata, podLabels, enableAutoMtls)
	// Don't add the annotations to the metadata - it's not documented so it's not coming
	// metadata = addAnnotations(metadata, addr.GetMetadata().GetAnnotations())

	if len(metadata.GetFilterMetadata()) == 0 {
		metadata = nil
	}

	return &envoy_config_endpoint_v3.LbEndpoint{
		Metadata: metadata,
		HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
			Endpoint: &envoy_config_endpoint_v3.Endpoint{
				Address: &envoy_config_core_v3.Address{
					Address: &envoy_config_core_v3.Address_SocketAddress{
						SocketAddress: &envoy_config_core_v3.SocketAddress{
							Protocol: envoy_config_core_v3.SocketAddress_TCP,
							Address:  address,
							PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
								PortValue: port,
							},
						},
					},
				},
			},
		},
	}
}

func addIstioAutomtlsMetadata(metadata *envoy_config_core_v3.Metadata, labels map[string]string, enableAutoMtls bool) *envoy_config_core_v3.Metadata {

	const EnvoyTransportSocketMatch = "envoy.transport_socket_match"
	if enableAutoMtls {
		if _, ok := labels[constants.IstioTlsModeLabel]; ok {
			metadata.GetFilterMetadata()[EnvoyTransportSocketMatch] = &structpb.Struct{
				Fields: map[string]*structpb.Value{
					constants.TLSModeLabelShortname: {
						Kind: &structpb.Value_StringValue{
							StringValue: constants.IstioMutualTLSModeLabel,
						},
					},
				},
			}
		}
	}
	return metadata
}

func createEndpoint(upstream *v1.Upstream) *envoy_config_endpoint_v3.ClusterLoadAssignment {
	clusterName := translator.UpstreamToClusterName(upstream.GetMetadata().Ref())
	return &envoy_config_endpoint_v3.ClusterLoadAssignment{
		ClusterName: getEndpointClusterName(clusterName, upstream),
	}
}

func findPortForService(kctx krt.HandlerContext, services krt.Collection[*corev1.Service], spec *kubeplugin.UpstreamSpec) (*corev1.ServicePort, bool) {
	maybeSvc := krt.FetchOne(kctx, services, krt.FilterObjectName(types.NamespacedName{
		Namespace: spec.GetServiceNamespace(),
		Name:      spec.GetServiceName(),
	}))
	if maybeSvc == nil {
		return nil, false
	}

	svc := *maybeSvc

	for _, port := range svc.Spec.Ports {
		if spec.GetServicePort() == uint32(port.Port) {
			return &port, len(svc.Spec.Ports) == 1
		}
	}

	return nil, false
}

func findFirstPortInEndpointSubsets(subset corev1.EndpointSubset, singlePortService bool, kubeServicePort *corev1.ServicePort) uint32 {
	var port uint32
	for _, p := range subset.Ports {
		// if the endpoint port is not named, it implies that
		// the kube service only has a single unnamed port as well.
		switch {
		case singlePortService:
			port = uint32(p.Port)
		case p.Name == kubeServicePort.Name:
			port = uint32(p.Port)
			break
		}
	}
	return port
}

// TODO: use exported version from translator?
func getEndpointClusterName(clusterName string, upstream *v1.Upstream) string {
	hash, err := upstream.Hash(nil)
	if err != nil {
		panic(err)
	}
	endpointClusterName := fmt.Sprintf("%s-%d", clusterName, hash)
	return endpointClusterName
}

// TODO: generalize this
func EnvoyCacheResourcesSetToFnvHash(resources []envoycache.Resource) uint64 {
	hasher := fnv.New64()
	var hash uint64
	// 8kb capacity, consider raising if we find the buffer is frequently being
	// re-allocated by MarshalAppend to fit larger protos.
	// the goal is to keep allocations constant for GC, without allocating an
	// unnecessarily large buffer.
	buffer := make([]byte, 0, 8*1024)
	mo := proto.MarshalOptions{Deterministic: true}
	for _, r := range resources {
		buf := buffer[:0]
		out, err := mo.MarshalAppend(buf, r.ResourceProto().(proto.Message))
		if err != nil {
			contextutils.LoggerFrom(context.Background()).DPanic(fmt.Errorf("marshalling envoy snapshot components: %w", err))
		}
		_, err = hasher.Write(out)
		if err != nil {
			contextutils.LoggerFrom(context.Background()).DPanic(fmt.Errorf("constructing hash for envoy snapshot components: %w", err))
		}
		hasher.Write([]byte{0})
		hash ^= hasher.Sum64()
		hasher.Reset()
	}
	return hash
}

// talk about settings doing an internal restart - we may not need it here with krt.
// and if we do, make sure that it works correctly with connected client set
// set locality loadbalancing priority - This is based on Region/Zone/SubZone matching.
func applyLocalityFailover(
	proxyLocality *envoy_config_core_v3.Locality,
	loadAssignment *envoy_config_endpoint_v3.ClusterLoadAssignment,
	failover []*v1alpha3.LocalityLoadBalancerSetting_Failover,
) {
	// key is priority, value is the index of the LocalityLbEndpoints in ClusterLoadAssignment
	priorityMap := map[int][]int{}

	// 1. calculate the LocalityLbEndpoints.Priority compared with proxy locality
	for i, localityEndpoint := range loadAssignment.Endpoints {
		// if region/zone/subZone all match, the priority is 0.
		// if region/zone match, the priority is 1.
		// if region matches, the priority is 2.
		// if locality not match, the priority is 3.
		priority := LbPriority(proxyLocality, localityEndpoint.Locality)
		// region not match, apply failover settings when specified
		// update localityLbEndpoints' priority to 4 if failover not match
		if priority == 3 {
			for _, failoverSetting := range failover {
				if failoverSetting.From == proxyLocality.Region {
					if localityEndpoint.Locality == nil || localityEndpoint.Locality.Region != failoverSetting.To {
						priority = 4
					}
					break
				}
			}
		}
		// priority is calculated using the already assigned priority using failoverPriority.
		// Since there are at most 5 priorities can be assigned using locality failover(0-4),
		// we multiply the priority by 5 for maintaining the priorities already assigned.
		// Afterwards the final priorities can be calculted from 0 (highest) to N (lowest) without skipping.
		priorityInt := int(loadAssignment.Endpoints[i].Priority*5) + priority
		loadAssignment.Endpoints[i].Priority = uint32(priorityInt)
		priorityMap[priorityInt] = append(priorityMap[priorityInt], i)
	}

	// since Priorities should range from 0 (highest) to N (lowest) without skipping.
	// 2. adjust the priorities in order
	// 2.1 sort all priorities in increasing order.
	priorities := []int{}
	for priority := range priorityMap {
		priorities = append(priorities, priority)
	}
	sort.Ints(priorities)
	// 2.2 adjust LocalityLbEndpoints priority
	// if the index and value of priorities array is not equal.
	for i, priority := range priorities {
		if i != priority {
			// the LocalityLbEndpoints index in ClusterLoadAssignment.Endpoints
			for _, index := range priorityMap[priority] {
				loadAssignment.Endpoints[index].Priority = uint32(i)
			}
		}
	}
}
func LbPriority(proxyLocality, endpointsLocality *envoy_config_core_v3.Locality) int {
	if proxyLocality.GetRegion() == endpointsLocality.GetRegion() {
		if proxyLocality.GetZone() == endpointsLocality.GetZone() {
			if proxyLocality.GetSubZone() == endpointsLocality.GetSubZone() {
				return 0
			}
			return 1
		}
		return 2
	}
	return 3
}
