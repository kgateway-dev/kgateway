package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/leaderelection"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
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

// XdsScopingMode selects backend resources for CDS, EDS, LDS and SDS.
// Backend-owned listeners and secrets follow their clusters; gateway-level
// listeners and secrets are unaffected.
type XdsScopingMode string

const (
	// XdsScopingAll emits every backend in discovery scope, referenced or not.
	// Its endpoints, backend-owned listeners and secrets are emitted too.
	// A retarget to an existing backend is safe because its cluster already
	// exists, but every proxy carries, and reports stats for, the whole
	// inventory.
	XdsScopingAll XdsScopingMode = "ALL"
	// XdsScopingReferenced emits only referenced clusters, including filter
	// dependencies (ext_authz, ext_proc, rate limit, access logs, JWKS), with their
	// endpoints, backend-owned listeners and secrets. A gateway
	// with a request-time selector reverts to ALL, since pruning could silently
	// select the plugin's fallback, unless every such selector is a cluster
	// specifier plugin that a valid pluginsdk.ClusterEmissionClaim accounts for.
	// Cluster headers, filter-state overrides, undecodable typed configs and
	// unknown specifier kinds cannot be claimed and always revert.
	XdsScopingReferenced XdsScopingMode = "REFERENCED"
)

// Decode implements envconfig.Decoder.
func (c *XdsScopingMode) Decode(value string) error {
	mode := XdsScopingMode(strings.ToUpper(value))
	switch mode {
	case XdsScopingAll, XdsScopingReferenced:
		*c = mode
		return nil
	default:
		return fmt.Errorf("invalid xDS scoping mode: %q", value)
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

// LabelSelector is a Kubernetes label selector in its string form, validated on decode.
// Empty selects everything.
type LabelSelector string

// Decode implements envconfig.Decoder.
func (l *LabelSelector) Decode(value string) error {
	if err := LabelSelector(value).Validate(); err != nil {
		return err
	}
	*l = LabelSelector(value)
	return nil
}

// Validate reports whether the selector parses. Settings built in code rather than decoded
// from the environment bypass Decode, so consumers that hand the selector to an informer
// call this too: the API server rejects a malformed selector on every List, which would keep
// the informer from ever syncing.
func (l LabelSelector) Validate() error {
	if _, err := labels.Parse(string(l)); err != nil {
		return fmt.Errorf("invalid label selector %q: %w", string(l), err)
	}
	return nil
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

	// IngressUseWaypoints enables the waypoint feature for ingress traffic.
	// When enabled, ingress traffic from ambient-captured gateways (the
	// ambient.istio.io/redirection=enabled annotation) to Services and ServiceEntries
	// labeled istio.io/ingress-use-waypoint=true is sent through their Istio waypoint.
	// The backend must have an Istio waypoint attached and a cluster IP: traffic goes
	// to the VIP so ztunnel can redirect it, and a headless Service gets a cluster with
	// no endpoints. See docs/guides/migrating-kgateway-waypoints.md.
	IngressUseWaypoints bool `split_words:"true" default:"false"`

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

	// XdsRespondOnReconnect answers a reconnecting proxy's first EDS, RDS and SDS
	// request even when it asks at the version it already holds. Without it, if
	// the controller has no snapshot for that proxy yet (after a restart, for
	// example), go-control-plane parks the request, and a rebuilt snapshot at the
	// same version never answers it. A cluster or listener warming on those
	// resources then waits for Envoy's fetch timeout, and a warming cluster keeps
	// CDS paused meanwhile. CDS and LDS requests are left alone, since nothing
	// warms on them. The cost is one EDS, RDS and SDS response, with content the
	// proxy already holds, per proxy that reconnects before its snapshot is
	// rebuilt.
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

	// ValidatorMode selects how strict validation executes unseen bootstraps.
	// Has no effect when ValidationMode is "STANDARD". Supported values:
	// - "BINARY": run envoy --mode validate for each submitted bootstrap.
	// - "CACHE": cache BINARY results in an LRU keyed by bootstrap content (default).
	//   Transient failures are not cached.
	//
	// In both modes, backend translation caches cluster verdicts by content before
	// building a bootstrap. Identical clusters therefore reuse a verdict in BINARY
	// mode too.
	ValidatorMode ValidatorMode `split_words:"true" default:"CACHE"`

	// ValidatorCacheSize sets the LRU capacity of the CACHE mode's bootstrap cache
	// and the translator's cluster verdict cache in every ValidatorMode.
	// A value <= 0 selects validator.DefaultCacheSize.
	ValidatorCacheSize int `split_words:"true"`

	// EnableBuiltinDefaultMetrics enables the default builtin controller-runtime metrics and go runtime metrics.
	// Since these metrics can be numerous, it is disabled by default.
	EnableBuiltinDefaultMetrics bool `split_words:"true" default:"false"`

	// GlobalPolicyNamespace is the namespace where policies that can attach to resources
	// in any namespace are defined.
	GlobalPolicyNamespace string `split_words:"true"`

	// Controls if leader election is disabled. Defaults to false.
	DisableLeaderElection bool `split_words:"true" default:"false"`

	// LeaderElectionLeaseDuration is the duration that non-leader candidates will wait
	// after observing a leadership renewal until attempting to acquire leadership of a
	// led but unrenewed leader slot. It must be greater than LeaderElectionRenewDeadline.
	// Only used when leader election is enabled (DisableLeaderElection is false).
	// Defaults to 15s, matching the controller-runtime default.
	LeaderElectionLeaseDuration time.Duration `split_words:"true" default:"15s"`

	// LeaderElectionRenewDeadline is the interval between attempts by the acting leader to
	// renew a leadership slot before it stops leading. It must be greater than
	// LeaderElectionRetryPeriod and less than LeaderElectionLeaseDuration.
	// Only used when leader election is enabled (DisableLeaderElection is false).
	// Defaults to 10s, matching the controller-runtime default.
	LeaderElectionRenewDeadline time.Duration `split_words:"true" default:"10s"`

	// LeaderElectionRetryPeriod is the duration the leader and non-leaders should wait
	// between attempting acquisition, and the leader between renewals, of leadership.
	// It must be less than LeaderElectionRenewDeadline.
	// Only used when leader election is enabled (DisableLeaderElection is false).
	// Defaults to 2s, matching the controller-runtime default.
	LeaderElectionRetryPeriod time.Duration `split_words:"true" default:"2s"`

	// EnableAwsEc2Discovery enables dynamic discovery of AWS EC2 instances for Backend resources.
	// This is disabled by default and must be explicitly enabled by the controller operator.
	EnableAwsEc2Discovery bool `split_words:"true" default:"false"`

	// AwsEc2RefreshInterval controls how often the controller refreshes EC2 instance discovery
	// for Backend resources when AWS EC2 discovery is enabled.
	AwsEc2RefreshInterval time.Duration `split_words:"true" default:"30s"`

	// TranslationDebounce batches changes to translation inputs before they trigger Gateway translation: changes
	// are delivered once no further change has arrived for this long. Each change otherwise rebuilds the whole
	// translation of every Gateway it affects, so batching a burst saves a rebuild per change, at the cost of
	// delaying each change by about this much. It currently applies to route changes. Set to 0 to deliver every
	// change immediately.
	TranslationDebounce time.Duration `split_words:"true" default:"10ms"`

	// TranslationDebounceMax bounds how long changes can be held while new changes keep arriving.
	// It is the main trade-off between translation work during a burst of changes and how stale
	// Gateway configuration and status can get. Values below TranslationDebounce are raised to it.
	TranslationDebounceMax time.Duration `split_words:"true" default:"100ms"`

	PolicyMerge string `split_words:"true" default:"{}"`

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

	// SecretLabelSelector restricts which Secrets kgateway watches to those matching this
	// Kubernetes label selector (for example "kgateway.dev/watch=true"). Empty, the default,
	// watches every Secret. The selector is pushed to the API server, so non-matching Secrets
	// never reach the informer cache. Every Secret referenced by Gateway API or kgateway
	// resources must match, otherwise the reference is reported as not found. The OAuth2 HMAC
	// Secret kgateway creates need not match: it is watched separately by name.
	SecretLabelSelector LabelSelector `split_words:"true"`

	// ConfigMapLabelSelector restricts which ConfigMaps kgateway watches, with the same
	// semantics as SecretLabelSelector. The per-proxy ConfigMaps kgateway renders need not
	// match: the gateway controller watches those separately.
	ConfigMapLabelSelector LabelSelector `split_words:"true"`

	// ServiceLabelSelector restricts which Services kgateway watches to those matching this
	// Kubernetes label selector (for example "kgateway.dev/watch=true" or "team in (a,b)").
	// Empty, the default, watches every Service. The selector is pushed to the API server, so
	// non-matching Services cost no memory, and because every watched Service becomes a
	// backend it also bounds the number of clusters kgateway sends to Envoy.
	//
	// Route backendRefs to a non-matching Service are reported as not found, and a
	// non-matching Service is not matched as a waypoint. The proxy Services kgateway renders
	// need not match: the gateway controller watches those separately.
	ServiceLabelSelector LabelSelector `split_words:"true"`

	// PerClientPublishBudget bounds how long per-client xDS publication may be
	// withheld while referenced clusters are not ready. A build whose referenced
	// clusters are all in CDS with derived endpoints publishes at once. Derived
	// endpoints may be empty, which is the backend's real state; endpoints not
	// yet derived are a gap.
	//
	//   - first publish: a client not yet published on its current connection
	//     gets the latest deferred snapshot at expiry, so a new gateway pod gets
	//     listeners instead of crash-looping; routes to unready clusters return
	//     503 until ready. The budget starts when the gate receives the
	//     client's first deferred build, not at connect; a client that left
	//     mid-withhold less than a budget earlier resumes its start. A warm
	//     client stays withheld while referenced clusters are missing from CDS,
	//     for at most PerClientWarmWithholdBound after expiry. If its only gaps
	//     are underived endpoints, it publishes at expiry and keeps the endpoints
	//     it holds: such gaps may never converge, and withholding would freeze
	//     its config.
	//   - flip release: a route flip held for a newly-referenced unready
	//     cluster publishes one budget after the hold began, so it cannot pin
	//     route, listener and secret updates indefinitely.
	//
	// A client is warm when its first listener request on the connection
	// carries a previously accepted version. That suggests, but does not prove,
	// that it still serves that config.
	//
	// The budget times installation in the controller's xDS cache. It is not a
	// deadline for a pod to go Ready, or for Envoy to receive or apply config:
	// translation before the first deferred build, KGW_XDS_FIRST_CONNECT_DELAY,
	// delivery, Envoy's fetch timeouts and cluster warming all add to it. Keep
	// it well below the proxy's startup probe window (60s by default); above
	// it, the first publish comes too late and the crash loop returns.
	//
	// 0 or a negative value removes both deadlines: an unpublished client waits
	// for a coherent build, and a held flip for its cluster, indefinitely.
	// PerClientWarmWithholdBound then has no effect. Timers that other settings
	// configure keep running.
	PerClientPublishBudget time.Duration `split_words:"true" default:"15s"`

	// PerClientWarmWithholdBound bounds how long a warm client (see
	// PerClientPublishBudget) stays withheld because referenced clusters are
	// missing from CDS. Without it, a cluster that never appears would freeze
	// the client's config, including certificate rotations, until the proxy
	// restarts. The withhold starts when the first-publish budget expires and
	// resumes if the client reconnects within the bound. At the bound the
	// latest deferred snapshot publishes, and routes to missing clusters fail
	// until they appear.
	//
	// It separately bounds how long a published client is still sent a missing
	// cluster's last config (make-before-break carry), so stale endpoints are
	// not served forever. Each carried cluster's clock starts with the first
	// build that lacks it and stops when it returns.
	//
	// Keep it well above how long translation takes to catch up after a
	// controller restart; a shorter bound fails warm routes on every restart.
	// Applies only while PerClientPublishBudget is positive. 0 or a negative
	// value removes the bound: warm clients stay withheld, and missing clusters
	// are carried, indefinitely.
	PerClientWarmWithholdBound time.Duration `split_words:"true" default:"10m"`

	// XdsSnapshotConsistencyCheck checks each per-client snapshot before
	// publish for EDS or RDS resources that no CDS or LDS resource references
	// (Envoy never requests them) and for listeners naming a missing
	// RouteConfiguration.
	// Violations are logged and counted in
	// kgateway_xds_snapshot_perclient_inconsistent_snapshots_total; any
	// increment is a kgateway bug. The snapshot still publishes: blocking would
	// reintroduce unbounded withholds. For test and CI; off by default.
	XdsSnapshotConsistencyCheck bool `split_words:"true" default:"false"`

	// XdsScopingMode selects which backend resources reach a proxy's CDS, EDS,
	// LDS and SDS. Backend-owned listeners and secrets follow their clusters;
	// gateway-level listeners and secrets are unaffected.
	// Supported values are:
	//
	//   - "ALL" (default): every backend in discovery scope becomes a cluster.
	//   - "REFERENCED": only clusters the generated configuration references.
	//
	// REFERENCED shrinks config_dump and per-proxy stats where many Services
	// are discovered and few are routed. It is experimental: Envoy does not
	// necessarily apply CDS before RDS within a snapshot, so a cluster entering
	// or leaving CDS could briefly 503 NC. XdsScopingReferenceAhead and
	// XdsScopingDereferenceGrace narrow the two transitions. Both are elapsed
	// time from publication, not acknowledgements, so each must exceed the fleet's
	// worst-case propagation, and neither makes a transition hitless. A gateway
	// whose routes pick a destination at request time stays unscoped unless a
	// valid claim accounts for each such selector (see
	// XdsScopingReferenced).
	XdsScopingMode XdsScopingMode `split_words:"true" default:"ALL"`

	// XdsScopingDereferenceGrace is how long a cluster that left the emitted set
	// stays published, in REFERENCED mode only. Removing it at once can let CDS remove it
	// before RDS updates the route still targeting it, causing 503 NC. The clock
	// starts when the controller publishes the route update that stops naming the
	// cluster, not when Envoy gets it. A build held for a new or unready cluster
	// does not start it; the release does. The grace gives that update time to land;
	// it does not confirm Envoy received or applied it, so it narrows the race
	// rather than closing it. Set it above the fleet's worst-case RDS propagation.
	// Only clusters scoping stops emitting are kept: a cluster whose backend was
	// deleted or fails translation leaves at once, as it does in ALL mode.
	// Negative values act as 0:
	// 0 removes immediately and accepts the race.
	XdsScopingDereferenceGrace time.Duration `split_words:"true" default:"5s"`

	// XdsScopingReferenceAhead is how long routes, listeners and secrets that name
	// a cluster new to a proxy are held after the controller first publishes
	// that cluster to it, in REFERENCED mode only. A cluster is new when the
	// proxy's published snapshot lacks it. Clusters only a filter calls count
	// too, such as a new ext_authz cluster.
	//
	// One snapshot carrying both is not enough. After a CDS response is sent
	// its watch stays closed until Envoy ACKs, so a route update in that window
	// goes out on the open RDS watch first. Any retarget during an un-ACKed CDS
	// update hits this, and no server option prevents it.
	//
	// The clock starts at the publish that first carries the cluster, just
	// before the controller installs it in its xDS cache. The send, Envoy's ACK,
	// and Envoy applying and warming the cluster are later events that nothing
	// observes. A send held back by an un-ACKed CDS response, or slow warming,
	// can outlast the window, so it narrows the race rather than closing it.
	//
	// Only edits that introduce a cluster the proxy's published snapshot lacks
	// pay the latency. 0 or a negative value publishes cluster and route
	// together and accepts that race. With PerClientPublishBudget positive, a
	// hold ends at most one budget after it began, so a misconfigured window or
	// a stream of new clusters cannot pin updates. With the budget at 0 or
	// less, a hold still ends when its windows close, but new clusters that
	// keep arriving can extend it.
	XdsScopingReferenceAhead time.Duration `split_words:"true" default:"2s"`

	// ReferenceGrantMode controls how cross-namespace references are validated via ReferenceGrant.
	// Supported values are:
	// - "OFF": No ReferenceGrant validation. All cross-namespace references are permitted.
	// - "PERMISSIVE": ReferenceGrant required for BackendRef and SecretRef (default behavior).
	// - "STRICT": ReferenceGrant required for all cross-namespace references including ExtensionRef.
	ReferenceGrantMode ReferenceGrantMode `split_words:"true" default:"PERMISSIVE"`
}

// renamedEnvVars pairs each env var a v2.5.0 beta read with the one that
// replaced it. envconfig ignores unknown variables, so without this check a
// setting under its old name would silently fall back to its default.
var renamedEnvVars = []struct{ from, to string }{
	{"KGW_CLUSTER_DISCOVERY_MODE", "KGW_XDS_SCOPING_MODE"},
	{"KGW_CLUSTER_REFERENCE_AHEAD", "KGW_XDS_SCOPING_REFERENCE_AHEAD"},
	{"KGW_CLUSTER_DEREFERENCE_GRACE", "KGW_XDS_SCOPING_DEREFERENCE_GRACE"},
}

// BuildSettings returns a zero-valued Settings obj if error is encountered when parsing env
func BuildSettings() (*Settings, error) {
	settings := &Settings{}
	var errs []error
	for _, r := range renamedEnvVars {
		if _, ok := os.LookupEnv(r.from); ok {
			errs = append(errs, fmt.Errorf("%s was renamed to %s; set %s instead", r.from, r.to, r.to))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return settings, err
	}
	if err := envconfig.Process("KGW", settings); err != nil {
		return settings, err
	}
	if err := applyDeprecatedEnableWaypoint(settings); err != nil {
		return settings, err
	}
	if err := settings.validateLeaderElection(); err != nil {
		return settings, err
	}
	return settings, nil
}

// validateLeaderElection mirrors the checks client-go runs when the leader elector is created,
// so that invalid durations fail when settings are loaded rather than when the manager starts.
func (s *Settings) validateLeaderElection() error {
	if s.DisableLeaderElection {
		return nil
	}
	if s.LeaderElectionLeaseDuration <= 0 || s.LeaderElectionRenewDeadline <= 0 || s.LeaderElectionRetryPeriod <= 0 {
		return fmt.Errorf("leader election durations must be greater than zero: leaseDuration=%s, renewDeadline=%s, retryPeriod=%s",
			s.LeaderElectionLeaseDuration, s.LeaderElectionRenewDeadline, s.LeaderElectionRetryPeriod)
	}
	if s.LeaderElectionLeaseDuration <= s.LeaderElectionRenewDeadline {
		return fmt.Errorf("leader election leaseDuration (%s) must be greater than renewDeadline (%s)",
			s.LeaderElectionLeaseDuration, s.LeaderElectionRenewDeadline)
	}
	if s.LeaderElectionRenewDeadline <= time.Duration(leaderelection.JitterFactor*float64(s.LeaderElectionRetryPeriod)) {
		return fmt.Errorf("leader election renewDeadline (%s) must be greater than retryPeriod (%s) * %v",
			s.LeaderElectionRenewDeadline, s.LeaderElectionRetryPeriod, leaderelection.JitterFactor)
	}
	return nil
}

var logger = logging.New("settings")

const (
	ingressUseWaypointsEnv = "KGW_INGRESS_USE_WAYPOINTS"
	// deprecatedEnableWaypointEnv used to turn on the waypoint plugin, which also
	// enabled ingress-use-waypoint while KGW_INGRESS_USE_WAYPOINTS defaulted to true.
	deprecatedEnableWaypointEnv = "KGW_ENABLE_WAYPOINT"
)

// applyDeprecatedEnableWaypoint keeps KGW_ENABLE_WAYPOINT=true working for
// deployments that don't set KGW_INGRESS_USE_WAYPOINTS, such as raw manifests or
// a new image running under an older chart. An explicit KGW_INGRESS_USE_WAYPOINTS
// always wins.
func applyDeprecatedEnableWaypoint(settings *Settings) error {
	val, ok := os.LookupEnv(deprecatedEnableWaypointEnv)
	if !ok {
		return nil
	}
	// Check the override before parsing, so an empty or malformed legacy value
	// can't block startup when the new variable is set.
	if _, set := os.LookupEnv(ingressUseWaypointsEnv); set {
		logger.Warn(deprecatedEnableWaypointEnv+" is deprecated and ignored because "+ingressUseWaypointsEnv+" is set", "value", val)
		return nil
	}
	enabled, err := strconv.ParseBool(val)
	if err != nil {
		return fmt.Errorf("envconfig.Process: assigning %s: converting '%s' to type bool: %w", deprecatedEnableWaypointEnv, val, err)
	}
	logger.Warn(deprecatedEnableWaypointEnv+" is deprecated; set "+ingressUseWaypointsEnv+" instead", "value", val)
	settings.IngressUseWaypoints = enabled
	return nil
}
