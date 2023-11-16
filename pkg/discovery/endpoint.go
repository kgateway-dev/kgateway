package discovery

import (
	"context"
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/solo-io/gloo/v2/pkg/translator/utils"
	"google.golang.org/protobuf/types/known/structpb"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const EnvoyLb = "envoy.lb"

type EndpointTranslator struct {
	UseVIP bool
}

// ComputeEndpointsForService computes the endpoints for Gloo from the given Kubernetes service.
// It returns the endpoints and warnings.
func (e *EndpointTranslator) ComputeEndpointsForService(
	ctx context.Context,
	service *corev1.Service,
	cli client.Client,
) ([]*endpointv3.ClusterLoadAssignment, []string) {
	var ret []*endpointv3.ClusterLoadAssignment
	var endpoints corev1.Endpoints
	err := cli.Get(ctx, client.ObjectKey{Name: service.Name, Namespace: service.Namespace}, &endpoints)
	// log err but continue with empty endpoints, so that service gets an empty list of endpoints.
	err = client.IgnoreNotFound(err)
	if err != nil {
		log.FromContext(ctx).Error(err, "failed to get endpoints for service", "name", service.Name, "ns", service.Namespace)
	}

	var warnsToLog []string

	// TODO: Investigate possible deprecation of ClusterIPs in newer k8s versions https://github.com/solo-io/gloo/issues/7830
	isHeadlessSvc := service.Spec.ClusterIP == "None"
	singlePortService := len(service.Spec.Ports) == 1
	// for each svc port
	for _, kubeServicePort := range service.Spec.Ports {
		var lbEndpoints []*endpointv3.LbEndpoint

		// Istio uses the service's port for routing requests
		// Headless services don't have a cluster IP, so we'll resort to pod IP endpoints
		if e.UseVIP && !isHeadlessSvc {
			lbEndpoints = append(lbEndpoints, createEndpoint(service.Spec.ClusterIP, uint32(kubeServicePort.Port), nil))
		} else {
			// find each matching endpoint
			for _, subset := range endpoints.Subsets {
				port := findFirstPortInEndpointSubsets(subset, singlePortService, &kubeServicePort)
				if port == 0 {
					warnsToLog = append(warnsToLog, fmt.Sprintf("port %v not found for service %v in endpoint %v", kubeServicePort.Port, service.Name, subset))
					continue
				}
				var warnings []string
				lbEndpoints, warnings = processSubsetAddresses(ctx, subset, port, cli)
				warnsToLog = append(warnsToLog, warnings...)
			}
		}

		clusterName := utils.ClusterName(service.Namespace, service.Name, kubeServicePort.Port)
		cla := &endpointv3.ClusterLoadAssignment{
			ClusterName: clusterName,
			Endpoints: []*endpointv3.LocalityLbEndpoints{{
				LbEndpoints: lbEndpoints,
			}},
		}
		ret = append(ret, cla)
	}

	return ret, warnsToLog
}

func processSubsetAddresses(ctx context.Context, subset corev1.EndpointSubset, port uint32, cli client.Client) ([]*endpointv3.LbEndpoint, []string) {
	var lbEndpoints []*endpointv3.LbEndpoint
	var warnings []string
	for _, addr := range subset.Addresses {
		var podLabels map[string]string
		var podName, podNamespace string
		targetRef := addr.TargetRef
		if targetRef != nil {
			if targetRef.Kind == "Pod" {
				podName = targetRef.Name
				podNamespace = targetRef.Namespace
			}
			var pod corev1.Pod
			// TODO: not sure we need to look by IP on new clusters with target refs.
			// if we do, add an indexer first in translator.go
			err := cli.Get(ctx, client.ObjectKey{Name: podName, Namespace: podNamespace}, &pod)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("error getting pod from ep target ref. pod: %s/%s. error: %s", podNamespace, podName, err))
			} else {
				podLabels = pod.Labels
			}
		}
		lbEndpoints = append(lbEndpoints, createEndpoint(addr.IP, port, podLabels))
	}
	return lbEndpoints, warnings
}

func findFirstPortInEndpointSubsets(subset corev1.EndpointSubset, singlePortService bool, kubeServicePort *corev1.ServicePort) uint32 {
	for _, p := range subset.Ports {
		// if the endpoint port is not named, it implies that
		// the kube service only has a single unnamed port as well.
		switch {
		case singlePortService:
			return uint32(p.Port)
		case p.Name == kubeServicePort.Name:
			return uint32(p.Port)
		}
	}
	return 0
}

func createEndpoint(address string, port uint32, labels map[string]string) *endpointv3.LbEndpoint {
	var metadata *corev3.Metadata
	if len(labels) > 0 {
		metadata = &corev3.Metadata{
			FilterMetadata: map[string]*structpb.Struct{},
		}
		if metadata.GetFilterMetadata() == nil {
			metadata.FilterMetadata = map[string]*structpb.Struct{}
		}

		labelsStruct := &structpb.Struct{
			Fields: map[string]*structpb.Value{},
		}

		for k, v := range labels {
			labelsStruct.GetFields()[k] = &structpb.Value{
				Kind: &structpb.Value_StringValue{
					StringValue: v,
				},
			}
		}
		metadata.GetFilterMetadata()[EnvoyLb] = labelsStruct
	}
	return &endpointv3.LbEndpoint{
		Metadata: metadata,
		HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
			Endpoint: &endpointv3.Endpoint{
				Address: &corev3.Address{
					Address: &corev3.Address_SocketAddress{
						SocketAddress: &corev3.SocketAddress{
							Protocol: corev3.SocketAddress_TCP,
							Address:  address,
							PortSpecifier: &corev3.SocketAddress_PortValue{
								PortValue: port,
							},
						},
					},
				},
			},
		},
	}
}
