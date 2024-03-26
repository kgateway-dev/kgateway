package api_conversion

import (
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var TestListenerBasicMetadata = &gloov1.Listener{
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
var TestListenerNoGateway = &gloov1.Listener{
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

var TestListenerMultipleGateways = &gloov1.Listener{
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
