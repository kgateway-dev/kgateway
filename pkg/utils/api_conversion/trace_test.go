package api_conversion_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/pkg/utils/api_conversion"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Trace utils", func() {
	Context("gets the gateway name from the parent listener", func() {
		DescribeTable("GetGatewayNameFromParent", func(listener *gloov1.Listener, expectedGatewayName string) {
			gatewayName := GetGatewayNameFromParent(context.TODO(), listener)
			Expect(gatewayName).To(Equal(expectedGatewayName))
		},
			Entry("listener with gateway",
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
										Name:      "gateway-name",
										Namespace: "gloo-system",
									},
									ResourceKind:       "*v1.Gateway",
									ObservedGeneration: 0,
								},
							},
						},
					},
				},
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
				UndefinedGatewayName,
			),
			Entry("listener with deprecated metadata",
				&gloov1.Listener{
					OpaqueMetadata: &gloov1.Listener_Metadata{},
				},
				DeprecatedMetadataGatewayName,
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
		)

	})

})
