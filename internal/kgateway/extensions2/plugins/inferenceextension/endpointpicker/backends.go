package endpointpicker

import (
	"context"
	"fmt"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	upstreamsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func processPoolBackendObjIR(
	ctx context.Context,
	in ir.BackendObjectIR,
	out *envoyclusterv3.Cluster,
	podIdx krt.Index[string, krtcollections.LocalityPod],
) *ir.EndpointsForBackend {
	// Build an endpoint list
	irPool := in.ObjIr.(*inferencePool)
	poolEps := irPool.resolvePoolEndpoints(podIdx)
	if len(poolEps) == 0 {
		logger.Warn("no endpoints resolved for InferencePool",
			"namespace", irPool.obj.GetNamespace(),
			"name", irPool.obj.GetName())
	}

	// If the pool has errors, create an empty LoadAssignment to return a 503
	if irPool.hasErrors() {
		logger.Debug("skipping endpoints due to InferencePool errors",
			"pool", in.ResourceName(),
			"errors", irPool.errors,
		)
		out.LoadAssignment = &envoyendpointv3.ClusterLoadAssignment{
			ClusterName: out.Name,
			Endpoints:   []*envoyendpointv3.LocalityLbEndpoints{{}},
		}
		return nil
	}

	// Static cluster with subset lb config
	out.Name = in.ClusterName()
	out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC}
	out.LbPolicy = envoyclusterv3.Cluster_ROUND_ROBIN
	out.LbSubsetConfig = &envoyclusterv3.Cluster_LbSubsetConfig{
		SubsetSelectors: []*envoyclusterv3.Cluster_LbSubsetConfig_LbSubsetSelector{{
			Keys: []string{dstEndpointKey},
		}},
		FallbackPolicy: envoyclusterv3.Cluster_LbSubsetConfig_ANY_ENDPOINT,
	}

	// TODO [danehans]: Set H1/H2 app protocol programmatically:
	// https://github.com/kubernetes-sigs/gateway-api-inference-extension/issues/1273
	addHTTP1(out)

	// Build the static LoadAssignment
	lbEndpoints := make([]*envoyendpointv3.LbEndpoint, 0, len(poolEps))
	for _, ep := range poolEps {
		addr := fmt.Sprintf("%s:%d", ep.address, ep.port)

		// Build the subset metadata struct used by the EPP for endpoint selection
		mdStruct, err := structpb.NewStruct(map[string]interface{}{
			dstEndpointKey: addr,
		})
		if err != nil {
			logger.Error("failed to build endpoint metadata for endpoint",
				"address", ep.address,
				"port", ep.port,
				"err", err)
			continue
		}

		// Build the LB endpoint
		lbEp := &envoyendpointv3.LbEndpoint{
			HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
				Endpoint: &envoyendpointv3.Endpoint{
					Address: &envoycorev3.Address{
						Address: &envoycorev3.Address_SocketAddress{
							SocketAddress: &envoycorev3.SocketAddress{
								Address:       ep.address,
								PortSpecifier: &envoycorev3.SocketAddress_PortValue{PortValue: uint32(ep.port)}, //nolint:gosec // G115: ep.port is int32 representing a port number, always in valid range
							},
						},
					},
				},
			},
			LoadBalancingWeight: wrapperspb.UInt32(1),
			Metadata: &envoycorev3.Metadata{
				FilterMetadata: map[string]*structpb.Struct{
					envoyLbNamespace: mdStruct,
				},
			},
		}
		lbEndpoints = append(lbEndpoints, lbEp)
	}

	// Attach the endpoints to the cluster load assignment
	out.LoadAssignment = &envoyendpointv3.ClusterLoadAssignment{
		ClusterName: out.Name,
		Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{
			LbEndpoints: lbEndpoints,
		}},
	}

	out.CircuitBreakers = &envoyclusterv3.CircuitBreakers{
		Thresholds: []*envoyclusterv3.CircuitBreakers_Thresholds{
			{
				MaxConnections:     wrapperspb.UInt32(defaultExtProcMaxRequests),
				MaxPendingRequests: wrapperspb.UInt32(defaultExtProcMaxRequests),
				MaxRequests:        wrapperspb.UInt32(defaultExtProcMaxRequests),
			},
		},
	}

	// Return nil since we're building a static cluster
	return nil
}

func addHTTP1(c *envoyclusterv3.Cluster) {
	http1Opts := &upstreamsv3.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreamsv3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &upstreamsv3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &upstreamsv3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{
					HttpProtocolOptions: &envoycorev3.Http1ProtocolOptions{},
				},
			},
		},
	}
	if anyH1, err := utils.MessageToAny(http1Opts); err == nil {
		c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
			"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": anyH1,
		}
	} else {
		logger.Error("failed to marshal HTTP/1 options", "err", err)
	}
}
