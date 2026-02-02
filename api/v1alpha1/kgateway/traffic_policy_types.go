package kgateway

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=trafficpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=trafficpolicies/status,verbs=get;update;patch

// +kubebuilder:printcolumn:name="Accepted",type=string,JSONPath=".status.ancestors[*].conditions[?(@.type=='Accepted')].status",description="Traffic policy acceptance status"
// +kubebuilder:printcolumn:name="Attached",type=string,JSONPath=".status.ancestors[*].conditions[?(@.type=='Attached')].status",description="Traffic policy attachment status"

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels="gateway.networking.k8s.io/policy=Direct"
type TrafficPolicy struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +required
	Spec TrafficPolicySpec `json:"spec"`
	// +optional
	Status gwv1.PolicyStatus `json:"status,omitempty"`
	// TODO: embed this into a typed Status field when
	// https://github.com/kubernetes/kubernetes/issues/131533 is resolved
}

// +kubebuilder:object:root=true
type TrafficPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TrafficPolicy `json:"items"`
}

// TrafficPolicySpec defines the desired state of a traffic policy.
// +kubebuilder:validation:XValidation:rule="!has(self.autoHostRewrite) || ((has(self.targetRefs) && self.targetRefs.all(r, r.kind == 'HTTPRoute')) || (has(self.targetSelectors) && self.targetSelectors.all(r, r.kind == 'HTTPRoute')))",message="autoHostRewrite can only be used when targeting HTTPRoute resources"
// +kubebuilder:validation:XValidation:rule="has(self.retry) && has(self.timeouts) ? (has(self.retry.perTryTimeout) && has(self.timeouts.request) ? duration(self.retry.perTryTimeout) < duration(self.timeouts.request) : true) : true",message="retry.perTryTimeout must be less than timeouts.request"
// +kubebuilder:validation:XValidation:rule="has(self.retry) && has(self.targetRefs) ? self.targetRefs.all(r, (r.kind == 'Gateway' ? has(r.sectionName) : true )) : true",message="targetRefs[].sectionName must be set when targeting Gateway resources with retry policy"
// +kubebuilder:validation:XValidation:rule="has(self.retry) && has(self.targetSelectors) ? self.targetSelectors.all(r, (r.kind == 'Gateway' ? has(r.sectionName) : true )) : true",message="targetSelectors[].sectionName must be set when targeting Gateway resources with retry policy"
type TrafficPolicySpec struct {
	// TargetRefs specifies the target resources by reference to attach the policy to.
	// +optional
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:XValidation:rule="self.all(r, (r.kind == 'Gateway' || r.kind == 'HTTPRoute' || r.kind.endsWith('ListenerSet')))",message="targetRefs may only reference Gateway, HTTPRoute, or ListenerSet resources"
	TargetRefs []shared.LocalPolicyTargetReferenceWithSectionName `json:"targetRefs,omitempty"`

	// TargetSelectors specifies the target selectors to select resources to attach the policy to.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.all(r, (r.kind == 'Gateway' || r.kind == 'HTTPRoute' || r.kind.endsWith('ListenerSet')))",message="targetSelectors may only reference Gateway, HTTPRoute, or ListenerSet resources"
	TargetSelectors []shared.LocalPolicyTargetSelectorWithSectionName `json:"targetSelectors,omitempty"`

	// Transformation is used to mutate and transform requests and responses
	// before forwarding them to the destination.
	// +optional
	Transformation *TransformationPolicy `json:"transformation,omitempty"`

	// ExtProc specifies the external processing configuration for the policy.
	// +optional
	ExtProc *ExtProcPolicy `json:"extProc,omitempty"`

	// ExtAuth specifies the external authentication configuration for the policy.
	// This controls what external server to send requests to for authentication.
	// +optional
	ExtAuth *ExtAuthPolicy `json:"extAuth,omitempty"`

	// RateLimit specifies the rate limiting configuration for the policy.
	// This controls the rate at which requests are allowed to be processed.
	// +optional
	RateLimit *RateLimit `json:"rateLimit,omitempty"`

	// Cors specifies the CORS configuration for the policy.
	// +optional
	Cors *CorsPolicy `json:"cors,omitempty"`

	// Csrf specifies the Cross-Site Request Forgery (CSRF) policy for this traffic policy.
	// +optional
	Csrf *CSRFPolicy `json:"csrf,omitempty"`

	// HeaderModifiers defines the policy to modify request and response headers.
	// +optional
	HeaderModifiers *shared.HeaderModifiers `json:"headerModifiers,omitempty"`

	// AutoHostRewrite rewrites the Host header to the DNS name of the selected upstream.
	// NOTE: This field is only honored for HTTPRoute targets.
	// NOTE: If `autoHostRewrite` is set on a route that also has a [URLRewrite filter](https://gateway-api.sigs.k8s.io/reference/spec/#httpurlrewritefilter)
	// configured to override the `hostname`, the `hostname` value will be used and `autoHostRewrite` will be ignored.
	// +optional
	AutoHostRewrite *bool `json:"autoHostRewrite,omitempty"`

	// Buffer can be used to set the maximum request size that will be buffered.
	// Requests exceeding this size will return a 413 response.
	// +optional
	Buffer *Buffer `json:"buffer,omitempty"`

	// Timeouts defines the timeouts for requests
	// It is applicable to HTTPRoutes and ignored for other targeted kinds.
	// +optional
	Timeouts *shared.Timeouts `json:"timeouts,omitempty"`

	// Retry defines the policy for retrying requests.
	// It is applicable to HTTPRoutes, Gateway listeners and XListenerSets, and ignored for other targeted kinds.
	// +optional
	Retry *Retry `json:"retry,omitempty"`

	// RBAC specifies the role-based access control configuration for the policy.
	// This defines the rules for authorization based on roles and permissions.
	// RBAC policies applied at different attachment points in the configuration
	// hierarchy are not cumulative, and only the most specific policy is enforced. This means an RBAC policy
	// attached to a route will override any RBAC policies applied to the gateway or listener.
	// +optional
	RBAC *shared.Authorization `json:"rbac,omitempty"`

	// JWT specifies the JWT authentication configuration for the policy.
	// This defines the JWT providers and their configurations.
	// +optional
	JWTAuth *JWTAuth `json:"jwtAuth,omitempty"`

	// UrlRewrite specifies URL rewrite rules for matching requests.
	// NOTE: This field is only honored for HTTPRoute targets.
	// +optional
	UrlRewrite *URLRewrite `json:"urlRewrite,omitempty"`

	// Compression configures response compression (per-route) and request/response
	// decompression (listener-level insertion triggered by route enable).
	// The response compression configuration is only honored for HTTPRoute targets.
	// +optional
	Compression *Compression `json:"compression,omitempty"`

	// BasicAuth specifies the HTTP basic authentication configuration for the policy.
	// This controls authentication using username/password credentials in the Authorization header.
	// +optional
	BasicAuth *BasicAuthPolicy `json:"basicAuth,omitempty"`

	// APIKeyAuth authenticates users based on a configured API Key.
	// +optional
	APIKeyAuth *APIKeyAuth `json:"apiKeyAuth,omitempty"`

	// OAuth2 specifies the configuration to use for OAuth2/OIDC.
	// Note: the OAuth2 filter does not protect against Cross-Site-Request-Forgery attacks on domains with cached
	// authentication (in the form of cookies). It is recommended to pair this with the CSRF policy to prevent
	// malicious social engineering.
	// +optional
	OAuth2 *OAuth2Policy `json:"oauth2,omitempty"`
}

// URLRewrite specifies URL rewrite rules using regular expressions.
// This allows more flexible and advanced path rewriting based on regex patterns.
// +kubebuilder:validation:AtLeastOneOf=pathRegex
type URLRewrite struct {
	// Path specifies the path rewrite configuration.
	// +optional
	PathRegex *PathRegexRewrite `json:"pathRegex,omitempty"`
}

// PathRegexRewrite specifies how to rewrite the URL path.
type PathRegexRewrite struct {
	// Pattern is the regex pattern that matches the URL path.
	// The pattern must be a valid RE2 regular expression.
	// If the HTTPRoute uses a RegularExpression path match, this field can use capture groups
	// from that match.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Pattern string `json:"pattern"`

	// Substitution is the replacement string for the matched pattern.
	// It can include backreferences to captured groups from the pattern (e.g., \1, \2)
	// or named groups (e.g., \g<name>).
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Substitution string `json:"substitution"`
}

// TransformationPolicy config is used to modify envoy behavior at a route level.
// These modifications can be performed on the request and response paths.
type TransformationPolicy struct {
	// Request is used to modify the request path.
	// +optional
	Request *Transform `json:"request,omitempty"`

	// Response is used to modify the response path.
	// +optional
	Response *Transform `json:"response,omitempty"`
}

// Transform defines the operations to be performed by the transformation.
// These operations may include changing the actual request/response but may also cause side effects.
// Side effects may include setting info that can be used in future steps (e.g. dynamic metadata) and can cause envoy to buffer.
type Transform struct {
	// Set is a list of headers and the value they should be set to.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=16
	Set []HeaderTransformation `json:"set,omitempty"`

	// Add is a list of headers to add to the request and what that value should be set to.
	// If there is already a header with these values then append the value as an extra entry.
	// Add is not supported on arm64 build, see docs/guides/transformation.md for details
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=16
	Add []HeaderTransformation `json:"add,omitempty"`

	// Remove is a list of header names to remove from the request/response.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MaxItems=16
	Remove []string `json:"remove,omitempty"`

	// Body controls both how to parse the body and if needed how to set.
	// If empty, body will not be buffered.
	// +optional
	Body *BodyTransformation `json:"body,omitempty"`
}

type InjaTemplate string

// EnvoyHeaderName is the name of a header or pseudo header
// Based on gateway api v1.Headername but allows a singular : at the start
//
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=256
// +kubebuilder:validation:Pattern=`^:?[A-Za-z0-9!#$%&'*+\-.^_\x60|~]+$`
// +k8s:deepcopy-gen=false
type (
	HeaderName           string
	HeaderTransformation struct {
		// Name is the name of the header to interact with.
		// +required
		Name HeaderName `json:"name"`
		// Value is the Inja template to apply to generate the output value for the header.
		// +optional
		Value InjaTemplate `json:"value,omitempty"`
	}
)

// BodyparseBehavior defines how the body should be parsed
// If set to json and the body is not json then the filter will not perform the transformation.
// +kubebuilder:validation:Enum=AsString;AsJson
type BodyParseBehavior string

const (
	// BodyParseBehaviorAsString will parse the body as a string.
	BodyParseBehaviorAsString BodyParseBehavior = "AsString"
	// BodyParseBehaviorAsJSON will parse the body as a json object.
	BodyParseBehaviorAsJSON BodyParseBehavior = "AsJson"
)

// BodyTransformation controls how the body should be parsed and transformed.
type BodyTransformation struct {
	// ParseAs defines what auto formatting should be applied to the body.
	// This can make interacting with keys within a json body much easier if AsJson is selected.
	// +kubebuilder:default=AsString
	// +optional
	ParseAs BodyParseBehavior `json:"parseAs,omitempty"`

	// Value is the template to apply to generate the output value for the body.
	// Only Inja templates are supported.
	// +optional
	Value *InjaTemplate `json:"value,omitempty"`
}

// RateLimit defines a rate limiting policy.
type RateLimit struct {
	// Local defines a local rate limiting policy.
	// +optional
	Local *LocalRateLimitPolicy `json:"local,omitempty"`

	// Global defines a global rate limiting policy using an external service.
	// +optional
	Global *RateLimitPolicy `json:"global,omitempty"`
}

// LocalRateLimitPolicy represents a policy for local rate limiting.
// It defines the configuration for rate limiting using a token bucket mechanism.
type LocalRateLimitPolicy struct {
	// TokenBucket represents the configuration for a token bucket local rate-limiting mechanism.
	// It defines the parameters for controlling the rate at which requests are allowed.
	// +optional
	TokenBucket *TokenBucket `json:"tokenBucket,omitempty"`

	// PercentEnabled specifies the percentage of requests for which the rate limiter is enabled.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	PercentEnabled *int32 `json:"percentEnabled,omitempty"`

	// PercentEnforced specifies the percentage of requests for which the rate limiter is enforced.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	PercentEnforced *int32 `json:"percentEnforced,omitempty"`
}

// TokenBucket defines the configuration for a token bucket rate-limiting mechanism.
// It controls the rate at which tokens are generated and consumed for a specific operation.
type TokenBucket struct {
	// MaxTokens specifies the maximum number of tokens that the bucket can hold.
	// This value must be greater than or equal to 1.
	// It determines the burst capacity of the rate limiter.
	// +required
	// +kubebuilder:validation:Minimum=1
	MaxTokens int32 `json:"maxTokens"`

	// TokensPerFill specifies the number of tokens added to the bucket during each fill interval.
	// If not specified, it defaults to 1.
	// This controls the steady-state rate of token generation.
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	TokensPerFill *int32 `json:"tokensPerFill,omitempty"`

	// FillInterval defines the time duration between consecutive token fills.
	// This value must be a valid duration string (e.g., "1s", "500ms").
	// It determines the frequency of token replenishment.
	// +required
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('50ms')",message="must be at least 50ms"
	FillInterval metav1.Duration `json:"fillInterval"`
}

// RateLimitPolicy defines a global rate limiting policy using an external service.
type RateLimitPolicy struct {
	// Descriptors define the dimensions for rate limiting.
	// These values are passed to the rate limit service which applies configured limits based on them.
	// Each descriptor represents a single rate limit rule with one or more entries.
	// +required
	// +kubebuilder:validation:MinItems=1
	Descriptors []RateLimitDescriptor `json:"descriptors"`

	// ExtensionRef references a GatewayExtension that provides the global rate limit service.
	// +required
	ExtensionRef shared.NamespacedObjectReference `json:"extensionRef"`
}

// RateLimitDescriptor defines a descriptor for rate limiting.
// A descriptor is a group of entries that form a single rate limit rule.
type RateLimitDescriptor struct {
	// Entries are the individual components that make up this descriptor.
	// When translated to Envoy, these entries combine to form a single descriptor.
	// +required
	// +kubebuilder:validation:MinItems=1
	Entries []RateLimitDescriptorEntry `json:"entries"`
}

// RateLimitDescriptorEntryType defines the type of a rate limit descriptor entry.
// +kubebuilder:validation:Enum=Generic;Header;RemoteAddress;Path
type RateLimitDescriptorEntryType string

const (
	// RateLimitDescriptorEntryTypeGeneric represents a generic key-value descriptor entry.
	RateLimitDescriptorEntryTypeGeneric RateLimitDescriptorEntryType = "Generic"

	// RateLimitDescriptorEntryTypeHeader represents a descriptor entry that extracts its value from a request header.
	RateLimitDescriptorEntryTypeHeader RateLimitDescriptorEntryType = "Header"

	// RateLimitDescriptorEntryTypeRemoteAddress represents a descriptor entry that uses the client's IP address as its value.
	RateLimitDescriptorEntryTypeRemoteAddress RateLimitDescriptorEntryType = "RemoteAddress"

	// RateLimitDescriptorEntryTypePath represents a descriptor entry that uses the request path as its value.
	RateLimitDescriptorEntryTypePath RateLimitDescriptorEntryType = "Path"
)

// RateLimitDescriptorEntry defines a single entry in a rate limit descriptor.
// Only one entry type may be specified.
// +kubebuilder:validation:XValidation:message="exactly one entry type must be specified",rule="(has(self.type) && (self.type == 'Generic' && has(self.generic) && !has(self.header)) || (self.type == 'Header' && has(self.header) && !has(self.generic)) || (self.type == 'RemoteAddress' && !has(self.generic) && !has(self.header)) || (self.type == 'Path' && !has(self.generic) && !has(self.header)))"
type RateLimitDescriptorEntry struct {
	// Type specifies what kind of rate limit descriptor entry this is.
	// +required
	Type RateLimitDescriptorEntryType `json:"type"`

	// Generic contains the configuration for a generic key-value descriptor entry.
	// This field must be specified when Type is Generic.
	// +optional
	Generic *RateLimitDescriptorEntryGeneric `json:"generic,omitempty"`

	// Header specifies a request header to extract the descriptor value from.
	// This field must be specified when Type is Header.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Header *string `json:"header,omitempty"`
}

// RateLimitDescriptorEntryGeneric defines a generic key-value descriptor entry.
type RateLimitDescriptorEntryGeneric struct {
	// Key is the name of this descriptor entry.
	// +required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`

	// Value is the static value for this descriptor entry.
	// +required
	// +kubebuilder:validation:MinLength=1
	Value string `json:"value"`
}

type CorsPolicy struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	*gwv1.HTTPCORSFilter `json:",inline"`

	// Disable the CORS filter.
	// Can be used to disable CORS policies applied at a higher level in the config hierarchy.
	// +optional
	Disable *shared.PolicyDisable `json:"disable,omitempty"`
}

// CSRFPolicy can be used to set percent of requests for which the CSRF filter is enabled,
// enable shadow-only mode where policies will be evaluated and tracked, but not enforced and
// add additional source origins that will be allowed in addition to the destination origin.
//
// +kubebuilder:validation:AtMostOneOf=percentageEnabled;percentageShadowed
type CSRFPolicy struct {
	// Specifies the percentage of requests for which the CSRF filter is enabled.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	PercentageEnabled *int32 `json:"percentageEnabled,omitempty"`

	// Specifies that CSRF policies will be evaluated and tracked, but not enforced.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	PercentageShadowed *int32 `json:"percentageShadowed,omitempty"`

	// Specifies additional source origins that will be allowed in addition to the destination origin.
	// +optional
	// +kubebuilder:validation:MaxItems=16
	AdditionalOrigins []shared.StringMatcher `json:"additionalOrigins,omitempty"`
}

// APIKeySource defines where to extract the API key from within a single key source.
// Within a single key source, if multiple types are specified, precedence is:
// header > query parameter > cookie. The header is checked first, and only falls back
// to query parameter if the header is not present, then to cookie if both header and query
// are not present.
// +kubebuilder:validation:AtLeastOneOf=header;query;cookie
type APIKeySource struct {
	// header specifies the name of the header that contains the API key.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Header *string `json:"header,omitempty"`

	// query specifies the name of the query parameter that contains the API key.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Query *string `json:"query,omitempty"`

	// cookie specifies the name of the cookie that contains the API key.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Cookie *string `json:"cookie,omitempty"`
}

// +kubebuilder:validation:ExactlyOneOf=secretRef;secretSelector;disable
type APIKeyAuth struct {
	// keySources specifies the list of key sources to extract the API key from.
	// Key sources are processed in array order and the first one that successfully
	// extracts a key is used. Within each key source, if multiple types (header, query, cookie) are
	// specified, precedence is: header > query parameter > cookie.
	//
	// If empty, defaults to a single key source with header "api-key".
	//
	// Example:
	//   keySources:
	//   - header: "X-API-KEY"
	//   - query: "api_key"
	//   - header: "Authorization"
	//     query: "token"
	//     cookie: "auth_token"
	//
	// In this example, the system will:
	// 1. First try header "X-API-KEY"
	// 2. If not found, try query parameter "api_key"
	// 3. If not found, try header "Authorization" (then query "token", then cookie "auth_token" within that key source)
	//
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=16
	// +optional
	KeySources []APIKeySource `json:"keySources,omitempty"`

	// forwardCredential controls whether the API key is included in the request sent to the upstream.
	// If false (default), the API key is removed from the request before sending to upstream.
	// If true, the API key is included in the request sent to upstream.
	// This applies to all configured key sources (header, query parameter, or cookie).
	// +optional
	ForwardCredential *bool `json:"forwardCredential,omitempty"`

	// clientIdHeader specifies the header name to forward the authenticated client identifier.
	// If not specified, the client identifier will not be forwarded in any header.
	// Example: "x-client-id"
	// +optional
	ClientIdHeader *string `json:"clientIdHeader,omitempty"`

	// secretRef references a Kubernetes secret storing a set of API Keys. If there are many keys, 'secretSelector' can be
	// used instead.
	//
	// Each entry in the Secret represents one API Key. The key is an arbitrary identifier.
	// The value is a string, representing the API Key.
	//
	// Example:
	//
	// apiVersion: v1
	// kind: Secret
	// metadata:
	//   name: api-key
	// stringData:
	//   client1: "k-123"
	//   client2: "k-456"
	//
	// +optional
	SecretRef *gwv1.SecretObjectReference `json:"secretRef,omitempty"`

	// secretSelector selects multiple secrets containing API Keys. If the same key is defined in multiple secrets, the
	// behavior is undefined.
	//
	// Each entry in the Secret represents one API Key. The key is an arbitrary identifier.
	// The value is a string, representing the API Key.
	//
	// Example:
	//
	// apiVersion: v1
	// kind: Secret
	// metadata:
	//   name: api-key
	// stringData:
	//   client1: "k-123"
	//   client2: "k-456"
	//
	// +optional
	SecretSelector *LabelSelector `json:"secretSelector,omitempty"`

	// Disable the API key authentication filter.
	// Can be used to disable API key authentication policies applied at a higher level in the config hierarchy.
	// +optional
	Disable *shared.PolicyDisable `json:"disable,omitempty"`
}

// LabelSelector selects resources using label selectors.
type LabelSelector struct {
	// Label selector to select the target resource.
	// +required
	MatchLabels map[string]string `json:"matchLabels"`
}

// +kubebuilder:validation:ExactlyOneOf=maxRequestSize;disable
type Buffer struct {
	// MaxRequestSize sets the maximum size in bytes of a message body to buffer.
	// Requests exceeding this size will receive HTTP 413.
	// Example format: "1Mi", "512Ki", "1Gi"
	// +optional
	// +kubebuilder:validation:XValidation:message="maxRequestSize must be greater than 0 and less than 4Gi",rule="(type(self) == int && int(self) > 0 && int(self) < 4294967296) || (type(self) == string && quantity(self).isGreaterThan(quantity('0')) && quantity(self).isLessThan(quantity('4Gi')))"
	MaxRequestSize *resource.Quantity `json:"maxRequestSize,omitempty"`

	// Disable the buffer filter.
	// Can be used to disable buffer policies applied at a higher level in the config hierarchy.
	// +optional
	Disable *shared.PolicyDisable `json:"disable,omitempty"`
}

// Compression configures HTTP gzip compression and decompression behavior.
// +kubebuilder:validation:AtLeastOneOf=responseCompression;requestDecompression
type Compression struct {
	// ResponseCompression controls response compression to the downstream.
	// If set, responses with the appropriate `Accept-Encoding` header with certain textual content types will be compressed using gzip.
	// The content-types that will be compressed are:
	// - `application/javascript`
	// - `application/json`
	// - `application/xhtml+xml`
	// - `image/svg+xml`
	// - `text/css`
	// - `text/html`
	// - `text/plain`
	// - `text/xml`
	// +optional
	ResponseCompression *ResponseCompression `json:"responseCompression,omitempty"`

	// RequestDecompression controls request decompression.
	// If set, gzip requests will be decompressed.
	// +optional
	RequestDecompression *RequestDecompression `json:"requestDecompression,omitempty"`
}

// ResponseCompression configures response compression.
type ResponseCompression struct {
	// Disables compression.
	// +optional
	Disable *shared.PolicyDisable `json:"disable,omitempty"`
}

// RequestDecompression enables request gzip decompression.
type RequestDecompression struct {
	// Disables decompression.
	// +optional
	Disable *shared.PolicyDisable `json:"disable,omitempty"`
}
