package krtcollections

import (
	"context"
	"fmt"
	"hash/fnv"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	ggv2utils "github.com/solo-io/gloo/projects/gateway2/utils"
	"github.com/solo-io/gloo/projects/gloo/constants"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/go-utils/contextutils"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
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
	Upstreams         krt.Collection[UpstreamWrapper]
	Endpoints         krt.Collection[*corev1.Endpoints]
	Pods              krt.Collection[LocalityPod]
	EndpointsSettings krt.Singleton[EndpointsSettings]
	Services          krt.Collection[*corev1.Service]
}

func NewGlooK8sEndpointInputs(
	settings krt.Singleton[glookubev1.Settings],
	istioClient kube.Client,
	pods krt.Collection[LocalityPod],
	services krt.Collection[*corev1.Service],
	finalUpstreams krt.Collection[UpstreamWrapper],
) EndpointsInputs {
	epClient := kclient.New[*corev1.Endpoints](istioClient)
	kubeEndpoints := krt.WrapClient(epClient, krt.WithName("Endpoints"))
	endpointSettings := krt.NewSingleton(func(ctx krt.HandlerContext) *EndpointsSettings {
		settings := krt.FetchOne(ctx, settings.AsCollection())
		return &EndpointsSettings{
			EnableAutoMtls: settings.Spec.GetGloo().GetIstioOptions().GetEnableAutoMtls().GetValue(),
		}
	})

	return EndpointsInputs{
		Upstreams:         finalUpstreams,
		Endpoints:         kubeEndpoints,
		Pods:              pods,
		EndpointsSettings: endpointSettings,
		Services:          services,
	}
}

type EndpointWithMd struct {
	*envoy_config_endpoint_v3.LbEndpoint
	EndpointMd EndpointMetadata
}
type EndpointsForUpstream struct {
	LbEps       map[PodLocality][]EndpointWithMd
	ClusterName string
	UpstreamRef types.NamespacedName
	Port        uint32
	Hostname    string
	clusterName string

	LbEpsEqualityHash uint64
}

func NewEndpointsForUpstream(us UpstreamWrapper, logger *zap.Logger) *EndpointsForUpstream {
	// start with a hash of the cluster name. technically we dont need it for krt, as we can compare the upstream name. but it helps later
	// to compute the hash we present envoy with.
	h := fnv.New64()
	h.Write([]byte(us.Inner.GetMetadata().Ref().String()))
	lbEpsEqualityHash := h.Sum64()

	// add the upstream hash to the clustername, so that if it changes the envoy cluster will become warm again.
	clusterName := GetEndpointClusterName(us.Inner)
	return &EndpointsForUpstream{
		LbEps:       make(map[PodLocality][]EndpointWithMd),
		ClusterName: clusterName,
		UpstreamRef: types.NamespacedName{
			Namespace: us.Inner.GetMetadata().GetNamespace(),
			Name:      us.Inner.GetMetadata().GetName(),
		},
		Port:              ggv2utils.GetPortForUpstream(us.Inner),
		Hostname:          ggv2utils.GetHostnameForUpstream(us.Inner),
		clusterName:       clusterName,
		LbEpsEqualityHash: lbEpsEqualityHash,
	}
}

func hashEndpoints(l PodLocality, emd EndpointWithMd) uint64 {
	hasher := fnv.New64()
	hasher.Write([]byte(l.Region))
	hasher.Write([]byte(l.Zone))
	hasher.Write([]byte(l.Subzone))

	ggv2utils.HashUint64(hasher, ggv2utils.HashLabels(emd.EndpointMd.Labels))
	ggv2utils.HashProtoWithHasher(hasher, emd.LbEndpoint)
	return hasher.Sum64()
}

func (e *EndpointsForUpstream) Add(l PodLocality, emd EndpointWithMd) {
	// xor it as we dont care about order - if we have the same endpoints in the same locality
	// we are good.
	e.LbEpsEqualityHash ^= hashEndpoints(l, emd)
	e.LbEps[l] = append(e.LbEps[l], emd)
}

func (c EndpointsForUpstream) ResourceName() string {
	return c.UpstreamRef.String()
}

func (c EndpointsForUpstream) Equals(in EndpointsForUpstream) bool {
	return c.UpstreamRef == in.UpstreamRef && c.Port == in.Port && c.LbEpsEqualityHash == in.LbEpsEqualityHash && c.Hostname == in.Hostname
}

func NewGlooK8sEndpoints(ctx context.Context, inputs EndpointsInputs) krt.Collection[EndpointsForUpstream] {
	return krt.NewCollection(inputs.Upstreams, transformK8sEndpoints(ctx, inputs), krt.WithName("GlooK8sEndpoints"))
}

func transformK8sEndpoints(ctx context.Context, inputs EndpointsInputs) func(kctx krt.HandlerContext, us UpstreamWrapper) *EndpointsForUpstream {
	augmentedPods := inputs.Pods
	kubeEndpoints := inputs.Endpoints
	services := inputs.Services

	logger := contextutils.LoggerFrom(ctx).Desugar()

	return func(kctx krt.HandlerContext, us UpstreamWrapper) *EndpointsForUpstream {
		var warnsToLog []string
		defer func() {
			for _, warn := range warnsToLog {
				logger.Warn(warn)
			}
		}()
		logger := logger.With(zap.String("upstream", us.Inner.GetMetadata().Ref().Key()))

		logger.Debug("building endpoints")

		kubeUpstream, ok := us.Inner.GetUpstreamType().(*v1.Upstream_Kube)
		// only care about kube upstreams
		if !ok {
			logger.Debug("not kube upstream")
			return nil
		}
		spec := kubeUpstream.Kube
		kubeServicePort, singlePortService := findPortForService(kctx, services, spec)
		if kubeServicePort == nil {
			logger.Debug("findPortForService - not found.", zap.Uint32("port", spec.GetServicePort()), zap.String("svcName", spec.GetServiceName()), zap.String("svcNamespace", spec.GetServiceNamespace()))
			return nil
		}

		maybeEps := krt.FetchOne(kctx, kubeEndpoints, krt.FilterObjectName(types.NamespacedName{
			Namespace: spec.GetServiceNamespace(),
			Name:      spec.GetServiceName(),
		}))
		if maybeEps == nil {
			warnsToLog = append(warnsToLog, fmt.Sprintf("endpoints not found for service %v", spec.GetServiceName()))
			logger.Debug("endpoints not found for service")
			return nil
		}
		eps := *maybeEps

		settings := krt.FetchOne(kctx, inputs.EndpointsSettings.AsCollection())
		enableAutoMtls := settings.EnableAutoMtls
		ret := NewEndpointsForUpstream(us, logger)
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
				ep := CreateLBEndpoint(addr.IP, port, augmentedLabels, enableAutoMtls)

				ret.Add(l, EndpointWithMd{
					LbEndpoint: ep,
					EndpointMd: EndpointMetadata{
						Labels: augmentedLabels,
					},
				})
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
func GetEndpointClusterName(upstream *v1.Upstream) string {
	clusterName := translator.UpstreamToClusterName(upstream.GetMetadata().Ref())
	endpointClusterName, err := translator.GetEndpointClusterName(clusterName, upstream)
	if err != nil {
		panic(err)
	}
	return endpointClusterName
}
