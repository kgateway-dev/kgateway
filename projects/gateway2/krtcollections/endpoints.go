package krtcollections

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"hash/fnv"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"knative.dev/pkg/network"

	"github.com/solo-io/gloo/projects/gateway2/ir"
	ggv2utils "github.com/solo-io/gloo/projects/gateway2/utils"
	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
	"github.com/solo-io/gloo/projects/gloo/constants"
	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"github.com/solo-io/go-utils/contextutils"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"
)

type EndpointMetadata struct {
	Labels map[string]string
}

type EndpointsSettings struct {
	EnableAutoMtls bool
}

var (
	_ krt.ResourceNamer              = EndpointsSettings{}
	_ krt.Equaler[EndpointsSettings] = EndpointsSettings{}
)

func (p EndpointsSettings) Equals(in EndpointsSettings) bool {
	return p == in
}

func (p EndpointsSettings) ResourceName() string {
	return "endpoints-settings"
}

type EndpointsInputs struct {
	// this is svc collection, other types will be ignored
	Upstreams               krt.Collection[ir.Upstream]
	EndpointSlices          krt.Collection[*discoveryv1.EndpointSlice]
	EndpointSlicesByService krt.Index[types.NamespacedName, *discoveryv1.EndpointSlice]
	Pods                    krt.Collection[LocalityPod]
	EndpointsSettings       krt.Singleton[EndpointsSettings]

	KrtOpts krtutil.KrtOptions
}

func NewGlooK8sEndpointInputs(
	settings krt.Singleton[glookubev1.Settings],
	istioClient kube.Client,
	krtopts krtutil.KrtOptions,
	pods krt.Collection[LocalityPod],
	k8supstreams krt.Collection[ir.Upstream],
) EndpointsInputs {
	epSliceClient := kclient.New[*discoveryv1.EndpointSlice](istioClient)
	endpointSlices := krt.WrapClient(epSliceClient, krtopts.ToOptions("EndpointSlices")...)
	endpointSettings := krt.NewSingleton(func(ctx krt.HandlerContext) *EndpointsSettings {
		settings := krt.FetchOne(ctx, settings.AsCollection())
		return &EndpointsSettings{
			EnableAutoMtls: settings.Spec.GetGloo().GetIstioOptions().GetEnableAutoMtls().GetValue(),
		}
	}, krtopts.ToOptions("EndpointSettings")...)

	// Create index on EndpointSlices by service name and endpointslice namespace
	endpointSlicesByService := krt.NewIndex(endpointSlices, func(es *discoveryv1.EndpointSlice) []types.NamespacedName {
		svcName, ok := es.Labels[discoveryv1.LabelServiceName]
		if !ok {
			return nil
		}
		return []types.NamespacedName{{
			Namespace: es.Namespace,
			Name:      svcName,
		}}
	})

	return EndpointsInputs{
		Upstreams:               k8supstreams,
		EndpointSlices:          endpointSlices,
		EndpointSlicesByService: endpointSlicesByService,
		Pods:                    pods,
		EndpointsSettings:       endpointSettings,
		KrtOpts:                 krtopts,
	}
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

type EndpointsForUpstream struct {
	LbEps LocalityLbMap
	// Note - in theory, cluster name should be a function of the UpstreamRef.
	// But due to an upstream envoy bug, the cluster name also includes the upstream hash.
	ClusterName string
	UpstreamRef ir.ObjectSource
	Port        uint32
	Hostname    string

	LbEpsEqualityHash uint64
	upstreamHash      uint64
	epsEqualityHash   uint64
}

func NewEndpointsForUpstream(us ir.Upstream, svc *corev1.Service, logger *zap.Logger) *EndpointsForUpstream {
	// start with a hash of the cluster name. technically we dont need it for krt, as we can compare the upstream name. but it helps later
	// to compute the hash we present envoy with.
	// add the upstream hash to the clustername, so that if it changes the envoy cluster will become warm again.

	h := fnv.New64a()
	h.Write([]byte(us.Group))
	h.Write([]byte{0})
	h.Write([]byte(us.Kind))
	h.Write([]byte{0})
	h.Write([]byte(us.Name))
	h.Write([]byte{0})
	h.Write([]byte(us.Namespace))
	upstreamHash := h.Sum64()

	return &EndpointsForUpstream{
		LbEps:             make(map[PodLocality][]EndpointWithMd),
		ClusterName:       us.ClusterName(),
		UpstreamRef:       us.ObjectSource,
		Port:              uint32(us.Port),
		Hostname:          network.GetServiceHostname(svc.Name, svc.Namespace),
		LbEpsEqualityHash: upstreamHash,
		upstreamHash:      upstreamHash,
	}
}

func hashEndpoints(l PodLocality, emd EndpointWithMd) uint64 {
	hasher := fnv.New64a()
	hasher.Write([]byte(l.Region))
	hasher.Write([]byte(l.Zone))
	hasher.Write([]byte(l.Subzone))

	ggv2utils.HashUint64(hasher, ggv2utils.HashLabels(emd.EndpointMd.Labels))
	ggv2utils.HashProtoWithHasher(hasher, emd.LbEndpoint)
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

func (e *EndpointsForUpstream) Add(l PodLocality, emd EndpointWithMd) {
	// xor it as we dont care about order - if we have the same endpoints in the same locality
	// we are good.
	e.epsEqualityHash ^= hashEndpoints(l, emd)
	// we can't xor the endpoint hash with the upstream hash, because upstreams with
	// different names and similar endpoints will cancel out, so endpoint changes
	// won't result in different equality hashes.
	e.LbEpsEqualityHash = hash(e.epsEqualityHash, e.upstreamHash)
	e.LbEps[l] = append(e.LbEps[l], emd)
}

func (c EndpointsForUpstream) ResourceName() string {
	return c.UpstreamRef.ResourceName()
}

func (c EndpointsForUpstream) Equals(in EndpointsForUpstream) bool {
	return c.UpstreamRef == in.UpstreamRef && c.ClusterName == in.ClusterName && c.Port == in.Port && c.LbEpsEqualityHash == in.LbEpsEqualityHash && c.Hostname == in.Hostname
}

func NewGlooK8sEndpoints(ctx context.Context, inputs EndpointsInputs) krt.Collection[EndpointsForUpstream] {
	return krt.NewCollection(inputs.Upstreams, transformK8sEndpoints(ctx, inputs), inputs.KrtOpts.ToOptions("GlooK8sEndpoints")...)
}

func transformK8sEndpoints(ctx context.Context, inputs EndpointsInputs) func(kctx krt.HandlerContext, us ir.Upstream) *EndpointsForUpstream {
	augmentedPods := inputs.Pods

	logger := contextutils.LoggerFrom(ctx).Desugar()

	return func(kctx krt.HandlerContext, us ir.Upstream) *EndpointsForUpstream {
		var warnsToLog []string
		defer func() {
			for _, warn := range warnsToLog {
				logger.Warn(warn)
			}
		}()
		key := types.NamespacedName{
			Namespace: us.Namespace,
			Name:      us.Name,
		}
		logger := logger.With(zap.Stringer("kubesvc", key))

		kubeUpstream, ok := us.Obj.(*corev1.Service)
		// only care about kube upstreams
		if !ok {
			logger.Debug("not kube upstream")
			return nil
		}

		logger.Debug("building endpoints")

		kubeSvcPort, singlePortSvc := findPortForService(kubeUpstream, uint32(us.Port))
		if kubeSvcPort == nil {
			logger.Debug("port not found for service", zap.Uint32("port", uint32(us.Port)))
			return nil
		}

		// Fetch all EndpointSlices for the upstream service

		endpointSlices := krt.Fetch(kctx, inputs.EndpointSlices, krt.FilterIndex(inputs.EndpointSlicesByService, key))
		if len(endpointSlices) == 0 {
			logger.Debug("no endpointslices found for service", zap.String("name", key.Name), zap.String("namespace", key.Namespace))
			return nil
		}

		// Handle potential eventually consistency of EndpointSlices for the upstream service
		found := false
		for _, endpointSlice := range endpointSlices {
			if port := findPortInEndpointSlice(endpointSlice, singlePortSvc, kubeSvcPort); port != 0 {
				found = true
				break
			}
		}
		if !found {
			logger.Debug("no ports found in endpointslices for service", zap.String("name", key.Name), zap.String("namespace", key.Namespace))
			return nil
		}

		// Initialize the returned EndpointsForUpstream
		settings := krt.FetchOne(kctx, inputs.EndpointsSettings.AsCollection())
		enableAutoMtls := settings.EnableAutoMtls
		ret := NewEndpointsForUpstream(us, kubeUpstream, logger)

		// Handle deduplication of endpoint addresses
		seenAddresses := make(map[string]struct{})

		// Add an endpoint to the returned EndpointsForUpstream for each EndpointSlice
		for _, endpointSlice := range endpointSlices {
			port := findPortInEndpointSlice(endpointSlice, singlePortSvc, kubeSvcPort)
			if port == 0 {
				logger.Debug("no port found in endpointslice; will try next endpointslice if one exists",
					zap.String("name", endpointSlice.Name),
					zap.String("namespace", endpointSlice.Namespace))
				continue
			}

			for _, endpoint := range endpointSlice.Endpoints {
				// Skip endpoints that are not ready
				if endpoint.Conditions.Ready != nil && !*endpoint.Conditions.Ready {
					continue
				}
				// Get the addresses
				for _, addr := range endpoint.Addresses {
					// Deduplicate addresses
					if _, exists := seenAddresses[addr]; exists {
						continue
					}
					seenAddresses[addr] = struct{}{}

					var podName string
					podNamespace := endpointSlice.Namespace
					targetRef := endpoint.TargetRef
					if targetRef != nil {
						if targetRef.Kind == "Pod" {
							podName = targetRef.Name
							if targetRef.Namespace != "" {
								podNamespace = targetRef.Namespace
							}
						}
					}

					var augmentedLabels map[string]string
					var l PodLocality
					if podName != "" {
						maybePod := krt.FetchOne(kctx, augmentedPods, krt.FilterObjectName(types.NamespacedName{
							Namespace: podNamespace,
							Name:      podName,
						}))
						if maybePod != nil {
							l = maybePod.Locality
							augmentedLabels = maybePod.AugmentedLabels
						}
					}
					ep := CreateLBEndpoint(addr, port, augmentedLabels, enableAutoMtls)

					ret.Add(l, EndpointWithMd{
						LbEndpoint: ep,
						EndpointMd: EndpointMetadata{
							Labels: augmentedLabels,
						},
					})
				}
			}
		}
		logger.Debug("created endpoint", zap.Int("numAddresses", len(ret.LbEps)))
		return ret
	}
}

func CreateLBEndpoint(address string, port uint32, podLabels map[string]string, enableAutoMtls bool) *envoy_config_endpoint_v3.LbEndpoint {
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
		Metadata:            metadata,
		LoadBalancingWeight: wrapperspb.UInt32(1),
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

func findPortForService(svc *corev1.Service, svcPort uint32) (*corev1.ServicePort, bool) {
	for _, port := range svc.Spec.Ports {
		if svcPort == uint32(port.Port) {
			return &port, len(svc.Spec.Ports) == 1
		}
	}

	return nil, false
}

func findPortInEndpointSlice(endpointSlice *discoveryv1.EndpointSlice, singlePortService bool, kubeServicePort *corev1.ServicePort) uint32 {
	var port uint32

	if endpointSlice == nil || kubeServicePort == nil {
		return port
	}

	for _, p := range endpointSlice.Ports {
		if p.Port == nil {
			continue
		}
		// If the endpoint port is not named, it implies that
		// the kube service only has a single unnamed port as well.
		switch {
		case singlePortService:
			port = uint32(*p.Port)
		case p.Name != nil && *p.Name == kubeServicePort.Name:
			port = uint32(*p.Port)
			break
		}
	}
	return port
}
