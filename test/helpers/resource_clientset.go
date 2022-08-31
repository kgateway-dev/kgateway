package helpers

import (
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

type ResourceClientSet interface {
	GatewayClient() gatewayv1.GatewayClient
	HttpGatewayClient() gatewayv1.MatchableHttpGatewayClient
	VirtualServiceClient() gatewayv1.VirtualServiceClient
	RouteTableClient() gatewayv1.RouteTableClient
	VirtualHostOptionClient() gatewayv1.VirtualHostOptionClient
	RouteOptionClient() gatewayv1.RouteOptionClient
	UpstreamGroupClient() gloov1.UpstreamGroupClient
	UpstreamClient() gloov1.UpstreamClient
	ProxyClient() gloov1.ProxyClient
	SecretClient() gloov1.SecretClient
	ArtifactClient() gloov1.ArtifactClient
}
