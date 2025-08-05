package settings

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

// RouteReplacementMode determines how invalid routes are handled during translation.
// Higher modes increase safety guarantees, but may have performance implications.
type RouteReplacementMode string

const (
	// RouteReplacementStandard rewrites invalid routes to direct responses
	// (typically HTTP 500), preserving a valid config while isolating failures.
	// This limits the blast radius of misconfigured routes or policies without
	// affecting unrelated tenants.
	RouteReplacementStandard RouteReplacementMode = "STANDARD"
	// RouteReplacementStrict builds on STANDARD by running targeted validation
	// (e.g. RDS, CDS, and security-related policies). Routes that fail these
	// checks are also replaced with direct responses, and helps prevent unsafe
	// config from reaching Envoy.
	RouteReplacementStrict RouteReplacementMode = "STRICT"
)

// Decode implements envconfig.Decoder.
func (m *RouteReplacementMode) Decode(value string) error {
	mode := RouteReplacementMode(value)
	switch mode {
	case RouteReplacementStandard, RouteReplacementStrict:
		*m = mode
		return nil
	default:
		return fmt.Errorf("invalid route replacement mode: %q", value)
	}
}

// DnsLookupFamily controls the DNS lookup family for all static clusters created via Backend resources.
type DnsLookupFamily string

const (
	// DnsLookupFamilyV4Preferred is the default value for DnsLookupFamily.
	// The DNS resolver will first perform a lookup for addresses in the IPv4 family
	// and fallback to a lookup for addresses in the IPv6 family. The callback target
	// will only get v6 addresses if there were no v4 addresses to return.
	DnsLookupFamilyV4Preferred DnsLookupFamily = "V4_PREFERRED"
	// DnsLookupFamilyV4Only is the value for DnsLookupFamily that only performs
	// DNS lookups for addresses in the IPv4 family.
	DnsLookupFamilyV4Only DnsLookupFamily = "V4_ONLY"
	// DnsLookupFamilyV6Only is the value for DnsLookupFamily that only performs
	// DNS lookups for addresses in the IPv6 family.
	DnsLookupFamilyV6Only DnsLookupFamily = "V6_ONLY"
	// DnsLookupFamilyAll is the value for DnsLookupFamily that performs lookups
	// for both IPv4 and IPv6 families and returns all resolved addresses.
	DnsLookupFamilyAll DnsLookupFamily = "ALL"
	// DnsLookupFamilyAuto is the value for DnsLookupFamily that first performs
	// a lookup for addresses in the IPv6 family and falls back to a lookup for
	// addresses in the IPv4 family. This is semantically equivalent to a
	// non-existent V6_PREFERRED option and is a legacy name that will be
	// deprecated in favor of V6_PREFERRED in a future major version.
	DnsLookupFamilyAuto DnsLookupFamily = "AUTO"
)

// Decode implements envconfig.Decoder.
func (m *DnsLookupFamily) Decode(value string) error {
	mode := DnsLookupFamily(value)
	switch mode {
	case DnsLookupFamilyV4Preferred, DnsLookupFamilyV4Only, DnsLookupFamilyV6Only, DnsLookupFamilyAll, DnsLookupFamilyAuto:
		*m = mode
		return nil
	default:
		return fmt.Errorf("invalid DNS lookup family: %q", value)
	}
}

type Settings struct {
	// Controls the DnsLookupFamily for all static clusters created via Backend resources.
	// If not set, kgateway will default to "V4_PREFERRED". Note that this is different
	// from the Envoy default of "AUTO", which is effectively "V6_PREFERRED".
	// Supported values are: "ALL", "AUTO", "V4_PREFERRED", "V4_ONLY", "V6_ONLY"
	// Details on the behavior of these options are available on the Envoy documentation:
	// https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto#enum-config-cluster-v3-cluster-dnslookupfamily
	DnsLookupFamily DnsLookupFamily `split_words:"true" default:"V4_PREFERRED"`

	// Controls the listener bind address. Can be either V4 or V6
	ListenerBindIpv6 bool `split_words:"true" default:"true"`

	EnableIstioIntegration bool `split_words:"true"`
	EnableIstioAutoMtls    bool `split_words:"true"`

	// IstioNamespace is the namespace where Istio control plane components are installed.
	// Defaults to "istio-system".
	IstioNamespace string `split_words:"true" default:"istio-system"`

	// XdsServiceHost is the host that serves xDS config.
	// It overrides xdsServiceName if set.
	XdsServiceHost string `split_words:"true"`

	// XdsServiceName is the name of the Kubernetes Service that serves xDS config.
	// It is assumed to be in the kgateway install namespace.
	// Ignored if XdsServiceHost is set.
	XdsServiceName string `split_words:"true" default:"kgateway"`

	// XdsServicePort is the port of the Kubernetes Service that serves xDS config.
	// This corresponds to the value of the `grpc-xds` port in the service.
	XdsServicePort uint32 `split_words:"true" default:"9977"`

	UseRustFormations bool `split_words:"true" default:"false"`

	// EnableInferExt defines whether to enable/disable support for Gateway API inference extension.
	EnableInferExt bool `split_words:"true"`
	// InferExtAutoProvision defines whether to enable/disable the Gateway API inference extension deployer.
	InferExtAutoProvision bool `split_words:"true"`

	// DefaultImageRegistry is the default image registry to use for the kgateway image.
	DefaultImageRegistry string `split_words:"true" default:"cr.kgateway.dev"`
	// DefaultImageTag is the default image tag to use for the kgateway image.
	DefaultImageTag string `split_words:"true" default:""`
	// DefaultImagePullPolicy is the default image pull policy to use for the kgateway image.
	DefaultImagePullPolicy string `split_words:"true" default:"IfNotPresent"`

	// WaypointLocalBinding will make the waypoint bind to a loopback address,
	// so that only the zTunnel can make connections to it. This requires the zTunnel
	// shipped with Istio 1.26.0+.
	WaypointLocalBinding bool `split_words:"true" default:"false"`

	// IngressUseWaypoints enables the waypoint feature for ingress traffic.
	// When enabled, backends with the ambient.istio.io/redirection=enabled annotation and
	// istio.io/ingress-use-waypoint=true label will be redirected through a waypoint proxy.
	// The feature is enabled by default and can be disabled by setting this to false.
	IngressUseWaypoints bool `split_words:"true" default:"true"`

	// LogLevel specifies the logging level (e.g., "trace", "debug", "info", "warn", "error").
	// Defaults to "info" if not set.
	LogLevel string `split_words:"true" default:"info"`

	// JSON representation of list of metav1.LabelSelector to select namespaces considered for resource discovery.
	// Defaults to an empty list which selects all namespaces.
	// E.g., [{"matchExpressions":[{"key":"kubernetes.io/metadata.name","operator":"In","values":["infra"]}]},{"matchLabels":{"app":"a"}}]
	DiscoveryNamespaceSelectors string `split_words:"true" default:"[]"`

	// EnableAgentGateway enables kgateway to send config to the agentgateway
	EnableAgentGateway bool `split_words:"true" default:"false"`

	// WeightedRoutePrecedence enables routes with a larger weight to take precedence over routes with a smaller weight.
	// If two routes have the same weight, Gateway API route precedence rules apply.
	// When enabled, the default weight for a route is 0.
	WeightedRoutePrecedence bool `split_words:"true" default:"false"`

	// RouteReplacementMode determines how invalid routes are handled during translation.
	// If not set, kgateway will default to "STANDARD". Supported values are:
	// - "STANDARD": Rewrites invalid routes to direct responses (typically HTTP 500)
	// - "STRICT": Builds on STANDARD by running targeted validation
	RouteReplacementMode RouteReplacementMode `split_words:"true" default:"STANDARD"`

	// EnableBuiltinDefaultMetrics enables the default builtin controller-runtime metrics and go runtime metrics.
	// Since these metrics can be numerous, it is disabled by default.
	EnableBuiltinDefaultMetrics bool `split_words:"true" default:"false"`

	// GlobalPolicyNamespace is the namespace where policies that can attach to resources
	// in any namespace are defined.
	GlobalPolicyNamespace string `split_words:"true"`
}

// BuildSettings returns a zero-valued Settings obj if error is encountered when parsing env
func BuildSettings() (*Settings, error) {
	settings := &Settings{}
	if err := envconfig.Process("KGW", settings); err != nil {
		return settings, err
	}
	return settings, nil
}
