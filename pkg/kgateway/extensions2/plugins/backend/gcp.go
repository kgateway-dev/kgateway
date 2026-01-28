package backend

import (
	"fmt"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	gcp_auth "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/gcp_authn/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

const (
	// gcpAuthnFilterName is the name of the GCP authn filter.
	gcpAuthnFilterName = "envoy.filters.http.gcp_authn"
	// gcpAuthnClusterName is the name of the GCP metadata cluster.
	gcpAuthnClusterName = "gcp_authn"
	// googleMetadataAddress is the default GCP metadata server address.
	googleMetadataAddress = "metadata.google.internal"
	// gcpBackendPort is the port used for GCP backends (always HTTPS).
	gcpBackendPort = uint32(443)
)

// GcpIr is the internal representation of a GCP backend.
type GcpIr struct {
	hostname          string
	transportSocket   *envoycorev3.TransportSocket
	audienceConfigAny *anypb.Any
}

// Equals checks if two GcpIr objects are equal.
func (u *GcpIr) Equals(other *GcpIr) bool {
	if u == nil || other == nil {
		return u == nil && other == nil
	}
	if u.hostname != other.hostname {
		return false
	}
	if !proto.Equal(u.transportSocket, other.transportSocket) {
		return false
	}
	if !proto.Equal(u.audienceConfigAny, other.audienceConfigAny) {
		return false
	}
	return true
}

// processGcp processes a GCP backend and returns an envoy cluster.
func processGcp(ir *GcpIr, out *envoyclusterv3.Cluster) error {
	if ir == nil {
		return fmt.Errorf("gcp ir is nil")
	}

	out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
		Type: envoyclusterv3.Cluster_STRICT_DNS,
	}

	if ir.transportSocket != nil {
		out.TransportSocket = ir.transportSocket
	}

	// Add audience config to cluster metadata
	if ir.audienceConfigAny != nil {
		if out.Metadata == nil {
			out.Metadata = &envoycorev3.Metadata{}
		}
		if out.Metadata.TypedFilterMetadata == nil {
			out.Metadata.TypedFilterMetadata = make(map[string]*anypb.Any)
		}
		out.Metadata.TypedFilterMetadata[gcpAuthnFilterName] = ir.audienceConfigAny
	}

	pluginutils.EnvoySingleEndpointLoadAssignment(out, ir.hostname, gcpBackendPort)
	return nil
}

// buildGcpIr builds the GCP IR from the backend specification.
func buildGcpIr(in *kgateway.GcpBackend) (*GcpIr, error) {
	hostname := in.Host

	// Build TLS transport socket with SNI
	typedConfig, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{
		Sni: hostname,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create tls context: %v", err)
	}
	transportSocket := &envoycorev3.TransportSocket{
		Name: "envoy.transport_sockets.tls",
		ConfigType: &envoycorev3.TransportSocket_TypedConfig{
			TypedConfig: typedConfig,
		},
	}

	// Build audience config (defaults to https://{host} if not specified)
	audienceURL := ptr.Deref(in.Audience, fmt.Sprintf("https://%s", hostname))
	audienceConfig := &gcp_auth.Audience{
		Url: audienceURL,
	}
	audienceConfigAny, err := utils.MessageToAny(audienceConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create audience config: %v", err)
	}

	return &GcpIr{
		hostname:          hostname,
		transportSocket:   transportSocket,
		audienceConfigAny: audienceConfigAny,
	}, nil
}

// getGcpAuthnCluster returns the GCP metadata cluster configuration.
func getGcpAuthnCluster() *envoyclusterv3.Cluster {
	return &envoyclusterv3.Cluster{
		Name:                 gcpAuthnClusterName,
		AltStatName:          gcpAuthnClusterName,
		ConnectTimeout:       &durationpb.Duration{Seconds: 5},
		DnsLookupFamily:      envoyclusterv3.Cluster_V4_ONLY,
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STRICT_DNS},
		RespectDnsTtl:        true,
		LoadAssignment: &envoyendpointv3.ClusterLoadAssignment{
			ClusterName: gcpAuthnClusterName,
			Endpoints: []*envoyendpointv3.LocalityLbEndpoints{
				{
					LbEndpoints: []*envoyendpointv3.LbEndpoint{
						{
							HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
								Endpoint: &envoyendpointv3.Endpoint{
									Address: &envoycorev3.Address{
										Address: &envoycorev3.Address_SocketAddress{
											SocketAddress: &envoycorev3.SocketAddress{
												Address: googleMetadataAddress,
												PortSpecifier: &envoycorev3.SocketAddress_PortValue{
													PortValue: 80,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// getGcpAuthnFilterConfig returns the GCP authn filter configuration.
func getGcpAuthnFilterConfig() *gcp_auth.GcpAuthnFilterConfig {
	return &gcp_auth.GcpAuthnFilterConfig{
		Cluster: gcpAuthnClusterName,
		Timeout: &durationpb.Duration{
			Seconds: 10,
		},
	}
}
