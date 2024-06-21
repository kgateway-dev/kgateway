package constants

import "github.com/solo-io/gloo/pkg/utils/helmutils"

const (
	GlooHelmRepoTemplate    = helmutils.RemoteChartUriTemplate
	GlooReleaseName         = "gloo"
	GlooFedReleaseName      = "gloo-fed"
	KnativeServingNamespace = "knative-serving"

	GlooFedDeploymentName = "gloo-fed"

	GlooContainerName = "gloo"

	Deployments        = "deployments"
	Pods               = "pods"
	Upstreams          = "upstreams"
	UpstreamGroup      = "upstreamgroup"
	AuthConfigs        = "auth-configs"
	RateLimitConfigs   = "rate-limit-configs"
	VirtualHostOptions = "virtual-host-options"
	RouteOptions       = "route-options"
	Secrets            = "secrets"
	VirtualServices    = "virtual-services"
	Gateways           = "gateways"
	Proxies            = "proxies"
	XDSMetrics         = "xds-metrics"
	KubeGatewayClasses = "kube-gateway-classes"
	KubeGateways       = "kube-gateways"
	KubeHTTPRoutes     = "kube-http-routes"
)

var (
	// This slice defines the valid prefixes for glooctl extension binaries within the user's PATH (e.g. "glooctl-foo").
	ValidExtensionPrefixes = []string{"glooctl"}
)
