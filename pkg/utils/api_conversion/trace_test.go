package api_conversion

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	envoytracegloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/trace/v3"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var basicListener = &gloov1.Listener{
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

var _ = Describe("Trace utils", func() {

	Context("gets the gateway name for the defined source", func() {
		DescribeTable("ToEnvoyOpenTelemetryConfiguration: serviceNamesource cases", func(glooOtelConfig *envoytracegloo.OpenTelemetryConfig, expectedGatewayName string) {
			listener := basicListener
			otelConfig, err := ToEnvoyOpenTelemetryConfiguration(context.TODO(), glooOtelConfig, "ClusterName", listener)
			Expect(err).NotTo(HaveOccurred())

			Expect(otelConfig.GetServiceName()).To(Equal(expectedGatewayName))
		},
			Entry("No ServiceNameSource set (use default)", &envoytracegloo.OpenTelemetryConfig{
				CollectorCluster: &envoytracegloo.OpenTelemetryConfig_CollectorUpstreamRef{
					CollectorUpstreamRef: &core.ResourceRef{
						Name:      "Name",
						Namespace: "Namespace",
					},
				},
			}, "gateway-name"),
			Entry("nil ServiceNameSource (user default)", &envoytracegloo.OpenTelemetryConfig{
				CollectorCluster: &envoytracegloo.OpenTelemetryConfig_CollectorUpstreamRef{
					CollectorUpstreamRef: &core.ResourceRef{
						Name:      "Name",
						Namespace: "Namespace",
					},
				},
				ServiceNameSource: nil,
			}, "gateway-name"),
			Entry("GatewayName ServiceNameSource", &envoytracegloo.OpenTelemetryConfig{
				CollectorCluster: &envoytracegloo.OpenTelemetryConfig_CollectorUpstreamRef{
					CollectorUpstreamRef: &core.ResourceRef{
						Name:      "Name",
						Namespace: "Namespace",
					},
				},
				ServiceNameSource: &envoytracegloo.OpenTelemetryConfig_ServiceNameSource{
					SourceType: &envoytracegloo.OpenTelemetryConfig_ServiceNameSource_GatewayName{},
				},
			}, "gateway-name"),
		)

		DescribeTable("ToEnvoyOpenTelemetryConfiguration: metadata cases", func(listener *gloov1.Listener, expectedGatewayName string) {
			glooOtelConfig := &envoytracegloo.OpenTelemetryConfig{
				CollectorCluster: &envoytracegloo.OpenTelemetryConfig_CollectorUpstreamRef{
					CollectorUpstreamRef: &core.ResourceRef{
						Name:      "Name",
						Namespace: "Namespace",
					},
				},
			}
			otelConfig, err := ToEnvoyOpenTelemetryConfiguration(context.TODO(), glooOtelConfig, "ClusterName", listener)
			Expect(err).NotTo(HaveOccurred())

			Expect(otelConfig.GetServiceName()).To(Equal(expectedGatewayName))
		},
			Entry("listener with gateway",
				basicListener,
				"gateway-name",
			),
			Entry("listener with no gateway",
				&gloov1.Listener{
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
				},
				UndefinedMetadataServiceName,
			),
			Entry("listener with deprecated metadata",
				&gloov1.Listener{
					OpaqueMetadata: &gloov1.Listener_Metadata{},
				},
				DeprecatedMetadataServiceName,
			),
			Entry("listener with multiple gateways",
				&gloov1.Listener{
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
				},
				"gateway-name-1,gateway-name-2",
			),
			Entry("nil listener", nil, NoListenerServiceName),
		)

	})
})
