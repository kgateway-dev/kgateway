package api_conversion

import (
	"context"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytrace "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Trace utils", func() {

	Context("gets the gateway name for the defined source", func() {

		DescribeTable("by calling GetGatewayNameFromParent", func(listener *gloov1.Listener, expectedGatewayName string) {
			gatewayName := GetGatewayNameFromParent(context.TODO(), listener)

			Expect(gatewayName).To(Equal(expectedGatewayName))
		},
			Entry("listener with gateway",
				testListenerBasicMetadata,
				"gateway-name",
			),
			Entry("listener with no gateway",
				testListenerNoGateway,
				UndefinedMetadataServiceName,
			),
			Entry("listener with deprecated metadata",
				&gloov1.Listener{
					OpaqueMetadata: &gloov1.Listener_Metadata{},
				},
				DeprecatedMetadataServiceName,
			),
			Entry("listener with multiple gateways",
				testListenerMultipleGateways,
				"gateway-name-1,gateway-name-2",
			),
			Entry("nil listener", nil, UnkownMetadataServiceName),
		)

	})

	Context("creates the OpenTelemetryConfig", func() {
		It("calling ToEnvoyOpenTelemetryConfiguration", func() {
			clusterName := "cluster-name"
			serviceName := "service-name"
			expectedConfig := &envoytrace.OpenTelemetryConfig{
				GrpcService: &envoy_config_core_v3.GrpcService{
					TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
							ClusterName: clusterName,
						},
					},
				},
				ServiceName: serviceName,
			}

			actutalConfig := ToEnvoyOpenTelemetryConfiguration(clusterName, serviceName)
			Expect(actutalConfig).To(Equal(expectedConfig))
		})
	})
})

var testListenerBasicMetadata = &gloov1.Listener{
	OpaqueMetadata: &gloov1.Listener_MetadataStatic{
		MetadataStatic: &gloov1.SourceMetadata{
			Sources: []*gloov1.SourceMetadata_SourceRef{
				{
					ResourceRef: &core.ResourceRef{
						Name:      "delegate-1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: &core.ResourceRef{
						Name:      "gateway-name",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.Gateway",
					ObservedGeneration: 0,
				},
			},
		},
	},
}
var testListenerNoGateway = &gloov1.Listener{
	OpaqueMetadata: &gloov1.Listener_MetadataStatic{
		MetadataStatic: &gloov1.SourceMetadata{
			Sources: []*gloov1.SourceMetadata_SourceRef{
				{
					ResourceRef: &core.ResourceRef{
						Name:      "delegate-1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
			},
		},
	},
}

var testListenerMultipleGateways = &gloov1.Listener{
	OpaqueMetadata: &gloov1.Listener_MetadataStatic{
		MetadataStatic: &gloov1.SourceMetadata{
			Sources: []*gloov1.SourceMetadata_SourceRef{
				{
					ResourceRef: &core.ResourceRef{
						Name:      "delegate-1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: &core.ResourceRef{
						Name:      "gateway-name-1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.Gateway",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: &core.ResourceRef{
						Name:      "gateway-name-2",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.Gateway",
					ObservedGeneration: 0,
				},
			},
		},
	},
}
