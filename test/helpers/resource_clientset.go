package helpers

import (
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	externalrl "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
)

type ResourceClientSet interface {
	GatewayClient() gatewayv1.GatewayClient
	HttpGatewayClient() gatewayv1.MatchableHttpGatewayClient
	TcpGatewayClient() gatewayv1.MatchableTcpGatewayClient
	VirtualServiceClient() gatewayv1.VirtualServiceClient
	RouteTableClient() gatewayv1.RouteTableClient
	VirtualHostOptionClient() gatewayv1.VirtualHostOptionClient
	RouteOptionClient() gatewayv1.RouteOptionClient
	SettingsClient() gloov1.SettingsClient
	UpstreamGroupClient() gloov1.UpstreamGroupClient
	UpstreamClient() gloov1.UpstreamClient
	ProxyClient() gloov1.ProxyClient
	AuthConfigClient() extauthv1.AuthConfigClient
	RateLimitConfigClient() externalrl.RateLimitConfigClient
	SecretClient() gloov1.SecretClient
	ArtifactClient() gloov1.ArtifactClient
}
