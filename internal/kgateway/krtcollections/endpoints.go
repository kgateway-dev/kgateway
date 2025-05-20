package krtcollections

import (
	"context"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

type EndpointsSettings struct {
	EnableAutoMtls bool
}

var (
	_      krt.ResourceNamer              = EndpointsSettings{}
	_      krt.Equaler[EndpointsSettings] = EndpointsSettings{}
	logger                                = logging.New("krtcollections")
)

func (p EndpointsSettings) Equals(in EndpointsSettings) bool {
	return p == in
}

func (p EndpointsSettings) ResourceName() string {
	return "endpoints-settings"
}

type EndpointsInputs struct {
	// this is svc collection, other types will be ignored
	Backends                krt.Collection[ir.BackendObjectIR]
	EndpointSlices          krt.Collection[*discoveryv1.EndpointSlice]
	EndpointSlicesByService krt.Index[types.NamespacedName, *discoveryv1.EndpointSlice]
	Pods                    krt.Collection[LocalityPod]
	EndpointsSettings       EndpointsSettings

	KrtOpts krtutil.KrtOptions
}

func NewGlooK8sEndpointInputs(
	stngs settings.Settings,
	krtopts krtutil.KrtOptions,
	endpointSlices krt.Collection[*discoveryv1.EndpointSlice],
	pods krt.Collection[LocalityPod],
	k8sBackends krt.Collection[ir.BackendObjectIR],
) EndpointsInputs {
	endpointSettings := EndpointsSettings{
		EnableAutoMtls: stngs.EnableIstioAutoMtls,
	}

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
		Backends:                k8sBackends,
		EndpointSlices:          endpointSlices,
		EndpointSlicesByService: endpointSlicesByService,
		Pods:                    pods,
		EndpointsSettings:       endpointSettings,
		KrtOpts:                 krtopts,
	}
}

func NewK8sEndpoints(ctx context.Context, inputs EndpointsInputs) krt.Collection[ir.EndpointsForBackend] {
	return krt.NewCollection(inputs.Backends, transformK8sEndpoints(ctx, inputs), inputs.KrtOpts.ToOptions("K8sEndpoints")...)
}

func transformK8sEndpoints(ctx context.Context, inputs EndpointsInputs) func(kctx krt.HandlerContext, backend ir.BackendObjectIR) *ir.EndpointsForBackend {
	augmentedPods := inputs.Pods

	return func(kctx krt.HandlerContext, backend ir.BackendObjectIR) *ir.EndpointsForBackend {
		var warnsToLog []string
		defer func() {
			for _, warn := range warnsToLog {
				logger.Warn(warn) //nolint:sloglint // ignore formatting
			}
		}()
		key := types.NamespacedName{
			Namespace: backend.Namespace,
			Name:      backend.Name,
		}
		kubeSvcLogger := logger.With("kubesvc", key)

		kubeBackend, ok := backend.Obj.(*corev1.Service)
		// only care about kube backend
		if !ok {
			kubeSvcLogger.Debug("not kube backend")
			return nil
		}

		kubeSvcLogger.Debug("building endpoints")

		kubeSvcPort, singlePortSvc := findPortForService(kubeBackend, uint32(backend.Port))
		if kubeSvcPort == nil {
			kubeSvcLogger.Debug("port not found for service", "port", backend.Port)
			return nil
		}

		// Fetch all EndpointSlices for the backend service
		endpointSlices := krt.Fetch(kctx, inputs.EndpointSlices, krt.FilterIndex(inputs.EndpointSlicesByService, key))
		if len(endpointSlices) == 0 {
			kubeSvcLogger.Debug("no endpointslices found for service", "name", key.Name, "namespace", key.Namespace)
			return nil
		}

		// Handle potential eventually consistency of EndpointSlices for the backend service
		found := false
		for _, endpointSlice := range endpointSlices {
			if port := findPortInEndpointSlice(endpointSlice, singlePortSvc, kubeSvcPort); port != 0 {
				found = true
				break
			}
		}
		if !found {
			kubeSvcLogger.Debug("no ports found in endpointslices for service", "name", key.Name, "namespace", key.Namespace)
			return nil
		}

		// Initialize the returned EndpointsForBackend
		enableAutoMtls := inputs.EndpointsSettings.EnableAutoMtls
		ret := ir.NewEndpointsForBackend(backend)

		// Handle deduplication of endpoint addresses
		seenAddresses := make(map[string]struct{})

		// Add an endpoint to the returned EndpointsForBackend for each EndpointSlice
		for _, endpointSlice := range endpointSlices {
			port := findPortInEndpointSlice(endpointSlice, singlePortSvc, kubeSvcPort)
			if port == 0 {
				kubeSvcLogger.Debug("no port found in endpointslice; will try next endpointslice if one exists",
					"name", endpointSlice.Name,
					"namespace", endpointSlice.Namespace)
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
					var l ir.PodLocality
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

					ret.Add(l, ir.EndpointWithMd{
						LbEndpoint: ep,
						EndpointMd: ir.EndpointMetadata{
							Labels: augmentedLabels,
						},
					})
				}
			}
		}
		kubeSvcLogger.Debug("created endpoint", "total_endpoints", len(ret.LbEps))
		return ret
	}
}

func CreateLBEndpoint(address string, port uint32, podLabels map[string]string, enableAutoMtls bool) *envoy_config_endpoint_v3.LbEndpoint {
	// Don't get the metadata labels and filter metadata for the envoy load balancer based on the backend, as this is not used
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
		if _, ok := labels[wellknown.IstioTlsModeLabel]; ok {
			metadata.GetFilterMetadata()[EnvoyTransportSocketMatch] = &structpb.Struct{
				Fields: map[string]*structpb.Value{
					wellknown.TLSModeLabelShortname: {
						Kind: &structpb.Value_StringValue{
							StringValue: wellknown.IstioMutualTLSModeLabel,
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
