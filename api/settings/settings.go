package settings

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ValidationMode determines how invalid routes and policies are handled during translation.
// Higher levels increase safety guarantees, but may have performance implications.
type ValidationMode string

const (
	// ValidationStandard rewrites invalid routes to direct responses
	// (typically HTTP 500), preserving a valid config while isolating failures.
	// This limits the blast radius of misconfigured routes or policies without
	// affecting unrelated tenants.
	ValidationStandard ValidationMode = "STANDARD"
	// ValidationStrict builds on standard by running targeted validation
	// (e.g. RDS, CDS, and security-related policies). Routes that fail these
	// checks are also replaced with direct responses, and helps prevent unsafe
	// config from reaching Envoy.
	// Strict Validation is not supported with Rustformation yet,
	// see docs/guides/transformation.md for details
	ValidationStrict ValidationMode = "STRICT"
)

// Decode implements envconfig.Decoder.
func (v *ValidationMode) Decode(value string) error {
	level := ValidationMode(strings.ToUpper(value))
	switch level {
	case ValidationStandard, ValidationStrict:
		*v = level
		return nil
	default:
		return fmt.Errorf("invalid validation mode: %q", value)
	}
}

// ValidatorMode selects the strict-validation execution strategy.
type ValidatorMode string

const (
	// ValidatorBinary forks envoy --mode validate per call.
	ValidatorBinary ValidatorMode = "BINARY"
	// ValidatorCache wraps ValidatorBinary with an LRU result cache.
	ValidatorCache ValidatorMode = "CACHE"
)

// Decode implements envconfig.Decoder.
func (v *ValidatorMode) Decode(value string) error {
	mode := ValidatorMode(strings.ToUpper(value))
	switch mode {
	case ValidatorBinary, ValidatorCache:
		*v = mode
		return nil
	default:
		return fmt.Errorf("invalid validator mode: %q", value)
	}
}

// ReferenceGrantMode controls how strictly cross-namespace references are validated
// via ReferenceGrant across the control plane.
type ReferenceGrantMode string

const (
	// ReferenceGrantOff disables all ReferenceGrant validation. All cross-namespace
	// references are permitted without any grant. Use only in environments where
	// namespace isolation is enforced by other means.
	ReferenceGrantOff ReferenceGrantMode = "OFF"

	// ReferenceGrantPermissive enforces ReferenceGrant for cross-namespace
	// BackendRef and SecretRef references (current default behavior). Cross-namespace
	// ExtensionRef references are permitted without a grant.
	ReferenceGrantPermissive ReferenceGrantMode = "PERMISSIVE"

	// ReferenceGrantStrict enforces ReferenceGrant for all cross-namespace references,
	// including ExtensionRef (e.g., TrafficPolicy referencing a GatewayExtension in
	// another namespace).
	ReferenceGrantStrict ReferenceGrantMode = "STRICT"
)

// Decode implements envconfig.Decoder.
func (r *ReferenceGrantMode) Decode(value string) error {
	mode := ReferenceGrantMode(strings.ToUpper(value))
	switch mode {
	case ReferenceGrantOff, ReferenceGrantPermissive, ReferenceGrantStrict:
		*r = mode
		return nil
	default:
		return fmt.Errorf("invalid reference grant mode: %q", value)
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

// GatewayClassParametersRefs maps GatewayClass names to ParametersReference
type GatewayClassParametersRefs map[string]*gwv1.ParametersReference

// Decode implements envconfig.Decoder
func (r *GatewayClassParametersRefs) Decode(value string) error {
	if value == "" {
		*r = nil
		return nil
	}

	// First parse as a simple map to validate name is present
	var simpleParsed map[string]struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Group     string `json:"group,omitempty"`
		Kind      string `json:"kind,omitempty"`
	}
	if err := json.Unmarshal([]byte(value), &simpleParsed); err != nil {
		return fmt.Errorf("invalid gateway class parameters refs: %w", err)
	}

	parsed := make(map[string]*gwv1.ParametersReference, len(simpleParsed))
	for className, ref := range simpleParsed {
		if strings.TrimSpace(ref.Name) == "" {
			return fmt.Errorf("gateway class %q parametersRef.name must be set", className)
		}
		if strings.TrimSpace(ref.Namespace) == "" {
			return fmt.Errorf("gateway class %q parametersRef.namespace must be set", className)
		}
		ns := gwv1.Namespace(ref.Namespace)
		paramsRef := &gwv1.ParametersReference{
			Name:      ref.Name,
			Namespace: &ns,
		}
		if ref.Group != "" {
			paramsRef.Group = gwv1.Group(ref.Group)
		}
		if ref.Kind != "" {
			paramsRef.Kind = gwv1.Kind(ref.Kind)
		}

		parsed[className] = paramsRef
	}

	*r = parsed
	return nil
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

	// AdminBindAddress controls which host the admin/debug server binds to.
	// The default loopback-only binding avoids exposing pprof, logging control,
	// and config snapshots outside the pod unless explicitly enabled.
	AdminBindAddress string `split_words:"true" default:"localhost"`

	EnableIstioIntegration bool `split_words:"true"`
	EnableIstioAutoMtls    bool `split_words:"true"`

	// IstioNamespace is the namespace where Istio control plane components are installed.
	// Defaults to "istio-system".
	IstioNamespace string `split_words:"true" default:"istio-system"`

	// WorkloadEntriesExclusionLabels is a comma-separated list of label keys. WorkloadEntries carrying
	// any of these label keys will be excluded from kgateway's endpoint discovery.
	WorkloadEntriesExclusionLabels string `split_words:"true"`

	// ServiceEntriesExclusionLabelSelectors is a JSON representation of a list of metav1.LabelSelector.
	// ServiceEntries matching any of these selectors will be excluded from kgateway's ServiceEntry backend
	// and endpoint discovery. Unlike WorkloadEntriesExclusionLabels, this uses full selectors so exclusions
	// can match specific label values.
	ServiceEntriesExclusionLabelSelectors string `split_words:"true" default:"[]"`

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

	// XdsAuth enables or disables xDS authentication between the data-plane and control-plane.
	// By default, this is enabled.
	XdsAuth bool `split_words:"true" default:"true"`

	// XdsTLS enables or disables TLS encryption for xDS communication between the data-plane and control-plane.
	// By default, this is disabled.
	XdsTLS bool `split_words:"true" default:"false"`

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

	// EnableOrderedAds delivers ADS responses to each Envoy strictly in the
	// snapshot cache's type order (CDS, EDS, LDS, RDS) instead of the default
	// randomized drain, closing the busy-stream reordering window on additions.
	// It does not change ACK-skew or removal ordering, because those depend on
	// when resource-type watches are open.
	EnableOrderedAds bool `split_words:"true" default:"true"`

	// XdsSuppressNackResend stops the control plane from re-sending an xDS
	// response the proxy has just rejected. After a NACK the proxy is still on
	// its previously accepted version, so the snapshot cache would otherwise
	// answer every NACK with the same rejected response and the two sides spin
	// at whatever rate the proxy can reject. With this enabled a NACK of the
	// current snapshot version parks the proxy's watch until the snapshot
	// changes; a NACK that arrives after the snapshot already changed is
	// answered immediately. Rejections are still logged and counted.
	XdsSuppressNackResend bool `split_words:"true" default:"true"`

	// XdsRespondOnReconnect answers a reconnecting proxy's first endpoint
	// request even when the proxy already holds the current version. After a
	// stream reset the proxy asks for endpoints at the version it accepted
	// before, and the snapshot cache answers only a different version, so a
	// cluster that was warming when the stream broke would otherwise wait for
	// Envoy's own endpoint fetch timeout before it could finish warming and let
	// CDS resume. The cost is one endpoint push per reconnecting proxy; other
	// resource types are not affected.
	XdsRespondOnReconnect bool `split_words:"true" default:"true"`

	// WeightedRoutePrecedence enables routes with a larger weight to take precedence over routes with a smaller weight.
	// If two routes have the same weight, Gateway API route precedence rules apply.
	// When enabled, the default weight for a route is 0.
	WeightedRoutePrecedence bool `split_words:"true" default:"false"`

	// ValidationMode determines how invalid routes and policies are handled during translation.
	// If not set, kgateway will default to "STANDARD". Supported values are:
	// - "STANDARD": Rewrites invalid routes to direct responses (typically HTTP 500)
	// - "STRICT": Builds on STANDARD by running targeted validation
	ValidationMode ValidationMode `split_words:"true" default:"STANDARD"`

	// ValidatorMode selects how the strict validator executes a bootstrap it has
	// not seen before. Has no effect when ValidationMode is "STANDARD". Supported
	// values:
	// - "BINARY": fork envoy --mode validate per bootstrap handed to the validator.
	// - "CACHE": wrap BINARY with an LRU result cache keyed on bootstrap content
	//   hash (default). A validation verdict is a pure function of the config
	//   bytes, so memoization cannot change outcomes, only skip redundant envoy
	//   invocations; transient failures are never cached.
	//
	// Independently of the mode, backend translation memoizes cluster verdicts by
	// the cluster's own content before any bootstrap is built, so a cluster that is
	// byte-identical to one already validated is not handed to the validator
	// again in either mode. That memo is what keeps per-client strict validation
	// affordable: every connected client's overlaid cluster is validated on every
	// walk over the backends, and nearly all of them repeat. BINARY therefore
	// means "fork envoy for every distinct cluster", not "for every call".
	ValidatorMode ValidatorMode `split_words:"true" default:"CACHE"`

	// ValidatorCacheSize is the LRU capacity of both the CACHE mode's bootstrap
	// cache and the translator's per-cluster verdict memo, which is sized from it
	// in every ValidatorMode. A value <= 0 (the default) selects the
	// implementation default, validator.DefaultCacheSize.
	ValidatorCacheSize int `split_words:"true"`

	// EnableBuiltinDefaultMetrics enables the default builtin controller-runtime metrics and go runtime metrics.
	// Since these metrics can be numerous, it is disabled by default.
	EnableBuiltinDefaultMetrics bool `split_words:"true" default:"false"`

	// GlobalPolicyNamespace is the namespace where policies that can attach to resources
	// in any namespace are defined.
	GlobalPolicyNamespace string `split_words:"true"`

	// Controls if leader election is disabled. Defaults to false.
	DisableLeaderElection bool `split_words:"true" default:"false"`

	// EnableAwsEc2Discovery enables dynamic discovery of AWS EC2 instances for Backend resources.
	// This is disabled by default and must be explicitly enabled by the controller operator.
	EnableAwsEc2Discovery bool `split_words:"true" default:"false"`

	// AwsEc2RefreshInterval controls how often the controller refreshes EC2 instance discovery
	// for Backend resources when AWS EC2 discovery is enabled.
	AwsEc2RefreshInterval time.Duration `split_words:"true" default:"30s"`

	PolicyMerge string `split_words:"true" default:"{}"`

	// EnableWaypoint enables kgateway to translate istio waypoints
	EnableWaypoint bool `split_words:"true" default:"false"`

	// EnableExperimentalGatewayAPIFeatures enables kgateway to support experimental features and APIs
	EnableExperimentalGatewayAPIFeatures bool `split_words:"true" default:"true"`

	// EnableRouteSourceMetadata enables attaching dev.kgateway.route_source filter metadata
	// to every Envoy route. This metadata includes the Kubernetes source object (kind, group,
	// name, namespace, rule) for each route, which can be useful for debugging and observability.
	// Disabled by default.
	//
	// Note: This feature is experimental and subject to breaking changes in future releases.
	EnableRouteSourceMetadata bool `split_words:"true" default:"false"`

	// GatewayClassParametersRefs configures the GatewayParameters references to set on the default GatewayClasses.
	// Format: JSON map where keys are GatewayClass names and values are objects with "name" (required),
	// "namespace" (required), "group" (optional), and "kind" (optional) fields.
	// E.g., {"gateway-class-name":{"name":"params-name","namespace":"params-namespace","group":"gateway.networking.k8s.io","kind":"GatewayParameters"}}
	GatewayClassParametersRefs GatewayClassParametersRefs `split_words:"true" default:"{}"`

	// Enables setting the `dev.kgateway.auth_policy:auth_succeeded=true` dynamic metadata on successfully-authenticated routes.
	EnableAuthMetadata bool `split_words:"true" default:"false"`

	// PerClientPublishBudget bounds how long per-client xDS publication may
	// be withheld while referenced clusters are not yet ready. It governs:
	//
	//   - first publish: a client that has NEVER been published a snapshot
	//     receives the latest deferred snapshot at expiry (it is always
	//     internally consistent), so a freshly scheduled gateway pod binds
	//     its listeners and becomes Ready instead of crash-looping; routes
	//     to still-unready clusters return 503 until they cohere. Clients
	//     that reported a prior accepted xDS version on connect are warm:
	//     they stay withheld while referenced clusters are missing from CDS,
	//     but publish at expiry when the only gaps are clusters whose
	//     endpoints were never derived (by then that is translation backlog
	//     or a plugin gap with no convergence guarantee, and withholding
	//     longer would freeze the client's config indefinitely).
	//   - flip release: a route flip held because it targets a
	//     newly-referenced cluster with no derived endpoints is published at
	//     expiry, so a reference that never becomes ready cannot pin the
	//     client's route/listener/secret updates indefinitely.
	//
	// Keep the budget well below the gateway proxy's startup probe window
	// (60s by default): a first publish bounded above the probe window
	// recreates the crash loop the bound exists to prevent. A value of 0
	// disables all bounds: clients wait for coherence with no deadline.
	PerClientPublishBudget time.Duration `split_words:"true" default:"15s"`

	// XdsSnapshotConsistencyCheck runs go-control-plane's Snapshot.Consistent()
	// on every per-client xDS snapshot immediately before it is published,
	// recording violations in the
	// kgateway_xds_snapshot_perclient_inconsistent_snapshots_total counter and
	// the error log. The snapshot is still published either way: the check is
	// an invariant monitor for test and CI environments (any increment is a
	// kgateway bug worth reporting), never a gate — withholding on
	// inconsistency would reintroduce the unbounded withholds the publication
	// engine removed. Off by default; enabled in e2e and conformance runs.
	XdsSnapshotConsistencyCheck bool `split_words:"true" default:"false"`

	// ReferenceGrantMode controls how cross-namespace references are validated via ReferenceGrant.
	// Supported values are:
	// - "OFF": No ReferenceGrant validation. All cross-namespace references are permitted.
	// - "PERMISSIVE": ReferenceGrant required for BackendRef and SecretRef (default behavior).
	// - "STRICT": ReferenceGrant required for all cross-namespace references including ExtensionRef.
	ReferenceGrantMode ReferenceGrantMode `split_words:"true" default:"PERMISSIVE"`
}

// BuildSettings returns a zero-valued Settings obj if error is encountered when parsing env
func BuildSettings() (*Settings, error) {
	settings := &Settings{}
	if err := envconfig.Process("KGW", settings); err != nil {
		return settings, err
	}
	return settings, nil
}
