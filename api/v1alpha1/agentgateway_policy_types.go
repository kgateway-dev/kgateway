package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=agentgatewaypolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=agentgatewaypolicies/status,verbs=get;update;patch

// +kubebuilder:printcolumn:name="Accepted",type=string,JSONPath=".status.ancestors[*].conditions[?(@.type=='Accepted')].status",description="Agentgateway policy acceptance status"
// +kubebuilder:printcolumn:name="Attached",type=string,JSONPath=".status.ancestors[*].conditions[?(@.type=='Attached')].status",description="Agentgateway policy attachment status"

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway,shortName=agpol
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels="gateway.networking.k8s.io/policy=Direct"
type AgentgatewayPolicy struct {
	metav1.TypeMeta `json:",inline"`
	// metadata for the object
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of AgentgatewayPolicy.
	// +required
	Spec AgentgatewayPolicySpec `json:"spec"`

	// status defines the current state of AgentgatewayPolicy.
	// +optional
	Status gwv1.PolicyStatus `json:"status,omitempty"`
	// TODO: embed this into a typed Status field when
	// https://github.com/kubernetes/kubernetes/issues/131533 is resolved
}

// +kubebuilder:object:root=true
type AgentgatewayPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentgatewayPolicy `json:"items"`
}

// A Common Expression Language (CEL) expression. See https://agentgateway.dev/docs/reference/cel/ for more info.
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=16384
// +k8s:deepcopy-gen=false
type CELExpression string

// +kubebuilder:validation:ExactlyOneOf=targetRefs;targetSelectors
// +kubebuilder:validation:XValidation:rule="has(self.traffic) || has(self.frontend) || has(self.backend)",message="At least one of traffic, frontend, or backend must be provided."
// +kubebuilder:validation:XValidation:rule="!has(self.backend) || !has(self.backend.mcp) || ((!has(self.targetRefs) || !self.targetRefs.exists(t, t.kind == 'Service')) && (!has(self.targetSelectors) || !self.targetSelectors.exists(t, t.kind == 'Service')))",message="backend.mcp may not be used with a Service target"
// +kubebuilder:validation:XValidation:rule="!has(self.backend) || !has(self.backend.ai) || ((!has(self.targetRefs) || !self.targetRefs.exists(t, t.kind == 'Service')) && (!has(self.targetSelectors) || !self.targetSelectors.exists(t, t.kind == 'Service')))",message="backend.ai may not be used with a Service target"
// +kubebuilder:validation:XValidation:rule="has(self.frontend) && has(self.targetRefs) ? self.targetRefs.all(t, t.kind == 'Gateway' && !has(t.sectionName)) : true",message="the 'frontend' field can only target a Gateway"
// +kubebuilder:validation:XValidation:rule="has(self.frontend) && has(self.targetSelectors) ? self.targetSelectors.all(t, t.kind == 'Gateway' && !has(t.sectionName)) : true",message="the 'frontend' field can only target a Gateway"
// +kubebuilder:validation:XValidation:rule="has(self.traffic) && has(self.targetRefs) ? self.targetRefs.all(t, t.kind in ['Gateway', 'HTTPRoute', 'XListenerSet']) : true",message="the 'traffic' field can only target a Gateway, XListenerSet, or HTTPRoute"
// +kubebuilder:validation:XValidation:rule="has(self.traffic) && has(self.targetSelectors) ? self.targetSelectors.all(t, t.kind in ['Gateway', 'HTTPRoute', 'XListenerSet']) : true",message="the 'traffic' field can only target a Gateway, XListenerSet, or HTTPRoute"
// +kubebuilder:validation:XValidation:rule="has(self.targetRefs) && has(self.traffic) && has(self.traffic.phase) && self.traffic.phase == 'PreRouting' ? self.targetRefs.all(t, t.kind in ['Gateway', 'XListenerSet']) : true",message="the 'traffic.phase=PreRouting' field can only target a Gateway or XListenerSet"
// +kubebuilder:validation:XValidation:rule="has(self.targetSelectors) && has(self.traffic) && has(self.traffic.phase) && self.traffic.phase == 'PreRouting' ? self.targetSelectors.all(t, t.kind in ['Gateway', 'XListenerSet']) : true",message="the 'traffic.phase=PreRouting' field can only target a Gateway or XListenerSet"
type AgentgatewayPolicySpec struct {
	// targetRefs specifies the target resources by reference to attach the policy to.
	//
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:XValidation:rule="self.all(r, (r.kind == 'Service' && r.group == '') || (r.kind == 'AgentgatewayBackend' && r.group == 'gateway.kgateway.dev') || (r.kind in ['Gateway', 'HTTPRoute'] && r.group == 'gateway.networking.k8s.io') || (r.kind == 'XListenerSet' && r.group == 'gateway.networking.x-k8s.io'))",message="targetRefs may only reference Gateway, HTTPRoute, XListenerSet, Service, or AgentgatewayBackend resources"
	// +kubebuilder:validation:XValidation:message="Only one Kind of targetRef can be set on one policy",rule="self.all(l1, !self.exists(l2, l1.kind != l2.kind))"
	// +optional
	TargetRefs []LocalPolicyTargetReferenceWithSectionName `json:"targetRefs,omitempty"`

	// targetSelectors specifies the target selectors to select resources to attach the policy to.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:XValidation:rule="self.all(r, (r.kind == 'Service' && r.group == '') || (r.kind == 'AgentgatewayBackend' && r.group == 'gateway.kgateway.dev') || (r.kind in ['Gateway', 'HTTPRoute'] && r.group == 'gateway.networking.k8s.io') || (r.kind == 'XListenerSet' && r.group == 'gateway.networking.x-k8s.io'))",message="targetRefs may only reference Gateway, HTTPRoute, XListenerSet, Service, or AgentgatewayBackend resources"
	// +kubebuilder:validation:XValidation:message="Only one Kind of targetRef can be set on one policy",rule="self.all(l1, !self.exists(l2, l1.kind != l2.kind))"
	// +optional
	TargetSelectors []LocalPolicyTargetSelectorWithSectionName `json:"targetSelectors,omitempty"`

	// frontend defines settings for how to handle incoming traffic.
	//
	// A frontend policy can only target a Gateway. Listener and ListenerSet are not valid targets.
	//
	// When multiple policies are selected for a given request, they are merged on a field-level basis, but not a deep
	// merge. For example, policy A sets 'tcp' and 'tls', and policy B sets 'tls', the effective policy would be 'tcp' from
	// policy A, and 'tls' from policy B.
	// +optional
	Frontend *AgentgatewayPolicyFrontend `json:"frontend,omitempty"`

	// traffic defines settings for how process traffic.
	//
	// A traffic policy can target a Gateway (optionally, with a sectionName indicating the listener), ListenerSet, Route
	// (optionally, with a sectionName indicating the route rule).
	//
	// When multiple policies are selected for a given request, they are merged on a field-level basis, but not a deep
	// merge. Precedence is given to more precise policies: Gateway < Listener < Route < Route Rule. For example, policy A
	// sets 'timeouts' and 'retries', and policy B sets 'retries', the effective policy would be 'timeouts' from policy A,
	// and 'retries' from policy B.
	// +optional
	Traffic *AgentgatewayPolicyTraffic `json:"traffic,omitempty"`

	// backend defines settings for how to connect to destination backends.
	//
	// A backend policy can target a Gateway (optionally, with a sectionName indicating the listener), ListenerSet, Route
	// (optionally, with a sectionName indicating the route rule), or a Service/Backend (optionally, with a sectionName
	// indicating the port (for Service) or sub-backend (for Backend).
	//
	// Note that a backend policy applies when connecting to a specific destination backend. Targeting a higher level
	// resource, like Gateway, is just a way to easily apply a policy to a group of backends.
	//
	// When multiple policies are selected for a given request, they are merged on a field-level basis, but not a deep
	// merge. Precedence is given to more precise policies: Gateway < Listener < Route < Route Rule < Backend/Service. For
	// example, if a Gateway policy sets 'tcp' and 'tls', and a Backend policy sets 'tls', the effective policy would be
	// 'tcp' from the Gateway, and 'tls' from the Backend.
	// +optional
	Backend *AgentgatewayPolicyBackendFull `json:"backend,omitempty"`
}

type AgentgatewayPolicyBackendSimple struct {
	// tcp defines settings for managing TCP connections to the backend.
	// +optional
	TCP *BackendTCP `json:"tcp,omitempty"`
	// tls defines settings for managing TLS connections to the backend.
	//
	// If this field is set, TLS will be initiated to the backend; the system trusted CA certificates will be used to
	// validate the server, and the SNI will automatically be set based on the destination.
	// +optional
	TLS *BackendTLS `json:"tls,omitempty"`
	// http defines settings for managing HTTP requests to the backend.
	// +optional
	HTTP *BackendHTTP `json:"http,omitempty"`

	// auth defines settings for managing authentication to the backend
	// +optional
	Auth *BackendAuth `json:"auth,omitempty"`
}

// +kubebuilder:validation:AtLeastOneOf=tcp;tls;http;auth;mcp
type AgentgatewayPolicyBackendMCP struct {
	AgentgatewayPolicyBackendSimple `json:",inline"`

	// mcp specifies settings for MCP workloads. This is only applicable when connecting to a Backend of type 'mcp'.
	// +optional
	MCP *BackendMCP `json:"mcp,omitempty"`
}

// +kubebuilder:validation:AtLeastOneOf=tcp;tls;http;auth;ai
type AgentgatewayPolicyBackendAI struct {
	AgentgatewayPolicyBackendSimple `json:",inline"`

	// ai specifies settings for AI workloads. This is only applicable when connecting to a Backend of type 'ai'.
	// +optional
	AI *BackendAI `json:"ai,omitempty"`
}

// +kubebuilder:validation:AtLeastOneOf=tcp;tls;http;auth;mcp;ai
type AgentgatewayPolicyBackendFull struct {
	AgentgatewayPolicyBackendSimple `json:",inline"`

	// ai specifies settings for AI workloads. This is only applicable when connecting to a Backend of type 'ai'.
	// +optional
	AI *BackendAI `json:"ai,omitempty"`

	// mcp specifies settings for MCP workloads. This is only applicable when connecting to a Backend of type 'mcp'.
	// +optional
	MCP *BackendMCP `json:"mcp,omitempty"`
}

// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=64
type TinyString = string

// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=256
type ShortString = string

// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=1024
type LongString = string

// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=253
// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
type SNI = string

type InsecureTLSMode string

const (
	// InsecureTLSModeInsecure disables all TLS verification
	InsecureTLSModeAll InsecureTLSMode = "All"
	// InsecureTLSModeHostname enables verifying the CA certificate, but disables verification of the hostname/SAN.
	// Note this is still, generally, very "insecure" as the name suggests.
	InsecureTLSModeHostname InsecureTLSMode = "Hostname"
)

// +kubebuilder:validation:AtMostOneOf=verifySubjectAltNames;insecureSkipVerify
// +kubebuilder:validation:XValidation:rule="has(self.insecureSkipVerify) && self.insecureSkipVerify == 'All' ? !has(self.caCertificateRefs) : true",message="insecureSkipVerify All and caCertificateRefs may not be set together"
// +kubebuilder:validation:XValidation:rule="has(self.insecureSkipVerify) ? !has(self.verifySubjectAltNames) : true",message="insecureSkipVerify and verifySubjectAltNames may not be set together"
type BackendTLS struct {
	// mtlsCertificateRef enables mutual TLS to the backend, using the specified key (tls.key) and cert (tls.crt) from the
	// refenced Secret.
	//
	// An optional 'ca.cert' field, if present, will be used to verify the server certificate if present. If
	// caCertificateRefs is also specified, the caCertificateRefs field takes priority.
	//
	// If unspecified, no client certificate will be used.
	//
	// TODO: must be secret
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=1
	// +optional
	MtlsCertificateRef []corev1.LocalObjectReference `json:"mtlsCertificateRef,omitempty"`
	// caCertificateRefs defines the CA certificate ConfigMap to use to verify the server certificate.
	// If unset, the system's trusted certificates are used.
	//
	// +listType=atomic
	// TODO: must be configmap
	// +kubebuilder:validation:MaxItems=1
	// +optional
	CACertificateRefs []corev1.LocalObjectReference `json:"caCertificateRefs,omitempty"`

	// insecureSkipVerify originates TLS but skips verification of the backend's certificate.
	// WARNING: This is an insecure option that should only be used if the risks are understood.
	//
	// There are two modes:
	// * All disables all TLS verification
	// * Hostname verifies the CA certificate is trusted, but ignores any mismatch of hostname/SANs. Note that this method
	//  is still insecure; prefer setting verifySubjectAltNames to customize the valid hostnames if possible.
	//
	// +kubebuilder:validation:Enum=All;Hostname
	// +optional
	InsecureSkipVerify *InsecureTLSMode `json:"insecureSkipVerify,omitempty"`

	// sni specifies the Server Name Indicator (SNI) to be used in the TLS handshake. If unset, the SNI is automatically
	// set based on the destination hostname.
	// +optional
	Sni *SNI `json:"sni,omitempty"`

	// verifySubjectAltNames specifies the Subject Alternative Names (SAN) to verify in the server certificate.
	// If not present, the destination hostname is automatically used.
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +optional
	VerifySubjectAltNames []ShortString `json:"verifySubjectAltNames,omitempty"`

	// alpnProtocols sets the Application Level Protocol Negotiation (ALPN) value to use in the TLS handshake.
	//
	// If not present, defaults to ["h2", "http/1.1"].
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +optional
	AlpnProtocols []TinyString `json:"alpnProtocols,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="!has(self.tracing)",message="tracing is not currently implemented"
type AgentgatewayPolicyFrontend struct {
	// tcp defines settings on managing incoming TCP connections.
	// +optional
	TCP *FrontendTCP `json:"tcp,omitempty"`
	// tls defines settings on managing incoming TLS connections.
	// +optional
	TLS *FrontendTLS `json:"tls,omitempty"`
	// http defines settings on managing incoming HTTP requests.
	// +optional
	HTTP *FrontendHTTP `json:"http,omitempty"`

	// AccessLoggingConfig contains access logging configuration
	// +optional
	AccessLog *AgentAccessLog `json:"accessLog,omitempty"`

	// Tracing contains various settings for OpenTelemetry tracer.
	// TODO: not currently implemented
	// +optional
	Tracing *AgentTracing `json:"tracing,omitempty"`
}

// +kubebuilder:validation:AtLeastOneOf=maxBufferSize;http1MaxHeaders;http1IdleTimeout;http2WindowSize;http2ConnectionWindowSize;http2FrameSize;http2KeepaliveInterval;http2KeepaliveTimeout
type FrontendHTTP struct {
	// maxBufferSize defines the maximum size HTTP body that will be buffered into memory.
	// Bodies will only be buffered for policies which require buffering.
	// If unset, this defaults to 2mb.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxBufferSize *int32 `json:"maxBufferSize,omitempty"`

	// http1MaxHeaders defines the maximum number of headers that are allowed in HTTP/1.1 requests.
	// If unset, this defaults to 100.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4096
	// +optional
	HTTP1MaxHeaders *int32 `json:"http1MaxHeaders,omitempty"`
	// http1IdleTimeout defines the timeout before an unused connection is closed.
	// If unset, this defaults to 10 minutes.
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1s')",message="http1IdleTimeout must be at least 1 second"
	// +optional
	HTTP1IdleTimeout *metav1.Duration `json:"http1IdleTimeout,omitempty"`

	// http2WindowSize indicates the initial window size for stream-level flow control for received data.
	// +kubebuilder:validation:Minimum=1
	// +optional
	HTTP2WindowSize *int32 `json:"http2WindowSize,omitempty"`
	// http2ConnectionWindowSize indicates the initial window size for connection-level flow control for received data.
	// +kubebuilder:validation:Minimum=1
	// +optional
	HTTP2ConnectionWindowSize *int32 `json:"http2ConnectionWindowSize,omitempty"`
	// http2FrameSize sets the maximum frame size to use.
	// If unset, this defaults to 16kb
	// +kubebuilder:validation:Minimum=16384
	// +kubebuilder:validation:Maximum=1677215
	// +optional
	HTTP2FrameSize *int32 `json:"http2FrameSize,omitempty"`
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1s')",message="http2KeepaliveInterval must be at least 1 second"
	// +optional
	HTTP2KeepaliveInterval *metav1.Duration `json:"http2KeepaliveInterval,omitempty"`
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1s')",message="http2KeepaliveTimeout must be at least 1 second"
	// +optional
	HTTP2KeepaliveTimeout *metav1.Duration `json:"http2KeepaliveTimeout,omitempty"`
}

// +kubebuilder:validation:AtLeastOneOf=handshakeTimeout
type FrontendTLS struct {
	// handshakeTimeout specifies the deadline for a TLS handshake to complete.
	// If unset, this defaults to 15s.
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('100ms')",message="handshakeTimeout must be at least 100ms"
	// +optional
	HandshakeTimeout *metav1.Duration `json:"handshakeTimeout,omitempty"`

	// TODO: mirror the tuneables on BackendTLS
}

// +kubebuilder:validation:AtLeastOneOf=keepalive
type FrontendTCP struct {
	// keepalive defines settings for enabling TCP keepalives on the connection.
	// +optional
	KeepAlive *AgentgatewayKeepalive `json:"keepalive,omitempty"`
}

// TCP Keepalive settings
type AgentgatewayKeepalive struct {
	// retries specifies the maximum number of keep-alive probes to send before dropping the connection.
	// If unset, this defaults to 9.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=64
	// +optional
	Retries *int32 `json:"retries,omitempty"`

	// time specifies the number of seconds a connection needs to be idle before keep-alive probes start being sent.
	// If unset, this defaults to 180s.
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1s')",message="time must be at least 1 second"
	// +optional
	Time *metav1.Duration `json:"time,omitempty"`

	// interval specifies the number of seconds between keep-alive probes.
	// If unset, this defaults to 180s.
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1s')",message="interval must be at least 1 second"
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`
}

// +kubebuilder:validation:Enum=PreRouting;PostRouting
type PolicyPhase string

const (
	PolicyPhasePreRouting  PolicyPhase = "PreRouting"
	PolicyPhasePostRouting PolicyPhase = "PostRouting"
)

// +kubebuilder:validation:XValidation:rule="has(self.phase) && self.phase == 'PreRouting' ? !has(self.rateLimit) && !has(self.cors) && !has(self.csrf) && !has(self.headerModifiers) && !has(self.hostRewrite) && !has(self.timeouts) && !has(self.retry) && !has(self.authorization): true",message="phase PreRouting only supports extAuth, transformation, and extProc"
type AgentgatewayPolicyTraffic struct {
	// The phase to apply the traffic policy to. If the phase is PreRouting, the targetRef must be a Gateway or a Listener.
	// PreRouting is typically used only when a policy needs to influence the routing decision.
	//
	// Even when using PostRouting mode, the policy can target the Gateway/Listener. This is a helper for applying the policy
	// to all routes under that Gateway/Listener, and follows the merging logic described above.
	//
	// Note: PreRouting and PostRouting rules do not merge together. These are independent execution phases. That is, all
	// PreRouting rules will merge and execute, then all PostRouting rules will merge and execute.
	//
	// If unset, this defaults to PostRouting.
	// +optional
	Phase *PolicyPhase `json:"phase,omitempty"` //nolint:kubeapilinter // false positive for the nophase sub-linter

	// transformation is used to mutate and transform requests and responses
	// before forwarding them to the destination.
	// +optional
	Transformation *AgentTransformationPolicy `json:"transformation,omitempty"`

	// extProc specifies the external processing configuration for the policy.
	// +optional
	ExtProc *AgentExtProcPolicy `json:"extProc,omitempty"`

	// extAuth specifies the external authentication configuration for the policy.
	// This controls what external server to send requests to for authentication.
	// +optional
	ExtAuth *AgentExtAuthPolicy `json:"extAuth,omitempty"`

	// rateLimit specifies the rate limiting configuration for the policy.
	// This controls the rate at which requests are allowed to be processed.
	// +optional
	RateLimit *AgentRateLimit `json:"rateLimit,omitempty"`

	// cors specifies the CORS configuration for the policy.
	// +optional
	Cors *AgentCorsPolicy `json:"cors,omitempty"`

	// csrf specifies the Cross-Site Request Forgery (CSRF) policy for this traffic policy.
	//
	// The CSRF policy has the following behavior:
	// * Safe methods (GET, HEAD, OPTIONS) are automatically allowed
	// * Requests without Sec-Fetch-Site or Origin headers are assumed to be same-origin or non-browser requests and are allowed.
	// * Otherwise, the Sec-Fetch-Site header is checked, with a fallback to comparing the Origin header to the Host header.
	// +optional
	Csrf *AgentCSRFPolicy `json:"csrf,omitempty"`

	// headerModifiers defines the policy to modify request and response headers.
	// +optional
	HeaderModifiers *HeaderModifiers `json:"headerModifiers,omitempty"`

	// hostRewrite specifies how to rewrite the Host header for requests.
	//
	// If the HTTPRoute `urlRewrite` filter already specifies a host rewrite, this setting is ignored.
	// +kubebuilder:validation:Enum=Auto;None
	// +optional
	HostnameRewrite *AgentHostnameRewriteConfig `json:"hostRewrite,omitempty"`

	// timeouts defines the timeouts for requests
	// It is applicable to HTTPRoutes and ignored for other targeted kinds.
	// +optional
	Timeouts *AgentTimeouts `json:"timeouts,omitempty"`

	// retry defines the policy for retrying requests.
	// +optional
	Retry *Retry `json:"retry,omitempty"`

	// authorization specifies the access rules based on roles and permissions.
	// If multiple authorization rules are applied across different policies (at the same, or different, attahcment points),
	// all rules are merged.
	// +optional
	Authorization *Authorization `json:"authorization,omitempty"`

	// jwtAuthentication authenticates users based on JWT tokens.
	// +optional
	JWTAuthentication *AgentJWTAuthentication `json:"jwtAuthentication,omitempty"`

	// basicAuthentication authenticates users based on the "Basic" authentication scheme (RFC 7617), where a username and password
	// are encoded in the request.
	// +optional
	BasicAuthentication *AgentBasicAuthentication `json:"basicAuthentication,omitempty"`

	// apiKeyAuthentication authenticates users based on a configured API Key.
	// +optional
	APIKeyAuthentication *AgentAPIKeyAuthentication `json:"apiKeyAuthentication,omitempty"`
}

// +kubebuilder:validation:Enum=Strict;Optional;Permissive
type JWTAuthenticationMode string

const (
	// A valid token, issued by a configured issuer, must be present.
	// This is the default option.
	JWTAuthenticationModeStrict JWTAuthenticationMode = "Strict"
	// If a token exists, validate it.
	// Warning: this allows requests without a JWT token!
	JWTAuthenticationModeOptional JWTAuthenticationMode = "Optional"
	// Requests are never rejected. This is useful for usage of claims in later steps (authorization, logging, etc).
	// Warning: this allows requests without a JWT token!
	JWTAuthenticationModePermissive JWTAuthenticationMode = "Permissive"
)

type AgentJWTAuthentication struct {
	// validation mode for JWT authentication.
	// +kubebuilder:default=Strict
	// +optional
	Mode JWTAuthenticationMode `json:"mode,omitempty"`
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=64
	// +required
	Providers []AgentJWTProvider `json:"providers"`
}

type AgentJWTProvider struct {
	// issuer identifies the IdP that issued the JWT. This corresponds to the 'iss' claim (https://tools.ietf.org/html/rfc7519#section-4.1.1).
	// +required
	Issuer ShortString `json:"issuer"`
	// audiences specifies the list of allowed audiences that are allowed access. This corresponds to the 'aud' claim (https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.3).
	// If unset, any audience is allowed.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=64
	// +optional
	Audiences []string `json:"audiences,omitempty"`
	// jwks defines the JSON Web Key Set used to validate the signature of the JWT.
	// +required
	JWKS AgentJWKS `json:"jwks"`
}

// +kubebuilder:validation:ExactlyOneOf=remote;inline
type AgentJWKS struct {
	// remote specifies how to reach the JSON Web Key Set from a remote address.
	// +optional
	Remote *AgentRemoteJWKS `json:"remote,omitempty"`
	// inline specifies an inline JSON Web Key Set used validate the signature of the JWT.
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=65536
	// +optional
	Inline *string `json:"inline,omitempty"`
}

// +kubebuilder:validation:ExactlyOneOf=uri;backendRef
type AgentRemoteJWKS struct {
	// IdP jwks endpoint. Default tls settings are used to connect to this url.
	// +kubebuilder:validation:Pattern=`^(https|http):\/\/[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*(:\d+)?\/.*$`
	// +optional
	JwksUri string `json:"uri,omitempty"`
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('5m')",message="cacheDuration must be at least 5m."
	// +kubebuilder:default="5m"
	CacheDuration *metav1.Duration `json:"cacheDuration,omitempty"`
	// backendRef references the remote JWKS server to reach.
	// Not implemented yet, only jwksUri is currently supported.
	// Supported types: Service and Backend.
	// +optional
	BackendRef gwv1.BackendObjectReference `json:"backendRef,omitempty"`
}

// +kubebuilder:validation:Enum=Strict;Optional
type BasicAuthenticationMode string

const (
	// A valid username and password must be present.
	// This is the default option.
	BasicAuthenticationModeStrict BasicAuthenticationMode = "Strict"
	// If a username and password exists, validate it.
	// Warning: this allows requests without a username!
	BasicAuthenticationModeOptional BasicAuthenticationMode = "Optional"
)

// +kubebuilder:validation:ExactlyOneOf=users;secretRef
type AgentBasicAuthentication struct {
	// validation mode for basic auth authentication.
	// +kubebuilder:default=Strict
	// +optional
	Mode BasicAuthenticationMode `json:"mode,omitempty"`

	// realm specifies the 'realm' to return in the WWW-Authenticate header for failed authentication requests.
	// If unset, "Restricted" will be used.
	// +optional
	Realm *string `json:"realm,omitempty"`

	// users provides an inline list of username/password pairs that will be accepted.
	// Each entry represents one line of the htpasswd format: https://httpd.apache.org/docs/2.4/programs/htpasswd.html.
	//
	// Note: passwords should be the hash of the password, not the raw password. Use the `htpasswd` or similar commands
	// to generate a hash. MD5, bcrypt, crypt, and SHA-1 are supported.
	//
	// Example:
	// users:
	// - "user1:$apr1$ivPt0D4C$DmRhnewfHRSrb3DQC.WHC."
	// - "user2:$2y$05$r3J4d3VepzFkedkd/q1vI.pBYIpSqjfN0qOARV3ScUHysatnS0cL2"
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=256
	// +optional
	Users []string `json:"users,omitempty"`

	// secretRef references a Kubernetes secret storing the .htaccess file. The Secret must have a key named '.htaccess',
	// and should contain the complete .htaccess file.
	//
	// Note: passwords should be the hash of the password, not the raw password. Use the `htpasswd` or similar commands
	// to generate a hash. MD5, bcrypt, crypt, and SHA-1 are supported.
	//
	// Example:
	//
	// apiVersion: v1
	// kind: Secret
	// metadata:
	//   name: basic-auth
	// stringData:
	//   .htaccess: |
	//     alice:$apr1$3zSE0Abt$IuETi4l5yO87MuOrbSE4V.
	//     bob:$apr1$Ukb5LgRD$EPY2lIfY.A54jzLELNIId/
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// +kubebuilder:validation:Enum=Strict;Optional
type APIKeyAuthenticationMode string

const (
	// A valid API Key must be present.
	// This is the default option.
	APIKeyAuthenticationModeStrict APIKeyAuthenticationMode = "Strict"
	// If an API Key exists, validate it.
	// Warning: this allows requests without an API Key!
	APIKeyAuthenticationModeOptional APIKeyAuthenticationMode = "Optional"
)

// +kubebuilder:validation:ExactlyOneOf=secretRef;secretSelector
type AgentAPIKeyAuthentication struct {
	// Validation mode for api key authentication.
	// +kubebuilder:default=Strict
	// +optional
	Mode APIKeyAuthenticationMode `json:"mode,omitempty"`

	// secretRef references a Kubernetes secret storing a set of API Keys. If there are many keys, 'secretSelector' can be
	// used instead.
	//
	// Each entry in the Secret represents one API Key. The key is an arbitrary identifier. The value can either be:
	// * A string, representing the API Key.
	// * A JSON object, with two fields, `key` and `metadata`. `key` contains the API Key. `metadata` contains arbitrary JSON
	//   metadata associated with the key, which may be used by other policies. For example, you may write an authorization
	//   policy allow `apiKey.group == 'sales'`.
	//
	// Example:
	//
	// apiVersion: v1
	// kind: Secret
	// metadata:
	//   name: api-key
	// stringData:
	//   client1: |
	//     {
	//       "key": "k-123",
	//       "metadata": {
	//         "group": "sales",
	//         "created_at": "2024-10-01T12:00:00Z",
	//       }
	//     }
	//   client2: "k-456"
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`

	// secretSelector selects multiple secrets containing API Keys. If the same key is defined in multiple secrets, the
	// behavior is undefined.
	//
	// Each entry in the Secret represents one API Key. The key is an arbitrary identifier. The value can either be:
	// * A string, representing the API Key.
	// * A JSON object, with two fields, `key` and `metadata`. `key` contains the API Key. `metadata` contains arbitrary JSON
	//   metadata associated with the key, which may be used by other policies. For example, you may write an authorization
	//   policy allow `apiKey.group == 'sales'`.
	//
	// Example:
	//
	// apiVersion: v1
	// kind: Secret
	// metadata:
	//   name: api-key
	// stringData:
	//   client1: |
	//     {
	//       "key": "k-123",
	//       "metadata": {
	//         "group": "sales",
	//         "created_at": "2024-10-01T12:00:00Z",
	//       }
	//     }
	//   client2: "k-456"
	// +optional
	SecretSelector *SecretSelector `json:"secretSelector,omitempty"`
}

type SecretSelector struct {
	// Label selector to select the target resource.
	// +required
	MatchLabels map[string]string `json:"matchLabels"`
}

type AgentHostnameRewrite string

const (
	AgentHostnameRewriteAuto AgentHostnameRewrite = "Auto"
	AgentHostnameRewriteNone AgentHostnameRewrite = "None"
)

// +kubebuilder:validation:ExactlyOneOf=key;secretRef;passthrough
type BackendAuth struct {
	// key provides an inline key to use as the value of the Authorization header.
	// This option is the least secure; usage of a Secret is preferred.
	// +kubebuilder:validation:MaxLength=2048
	// +optional
	InlineKey *string `json:"key,omitempty"`

	// secretRef references a Kubernetes secret storing the key to use the authorization value. This must be stored in the
	// 'Authorization' key.
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`

	// passthrough passes through an existing token that has been sent by the client and validated. Other policies, like
	// JWT and API Key authentication, will strip the original client credentials. Passthrough backend authentication
	// causes the original token to be added back into the request. If there are no client authentication policies on the
	// request, the original token would be unchanged, so this would have no effect.
	// +optional
	Passthrough *BackendAuthPassthrough `json:"passthrough,omitempty"`
	// TODO: aws, azure, gcp
}

type BackendAuthPassthrough struct {
}

// +kubebuilder:validation:AtLeastOneOf=prompt;promptGuard;defaults;overrides;modelAliases;promptCaching;routes
type BackendAI struct {
	// Enrich requests sent to the LLM provider by appending and prepending system prompts. This can be configured only for
	// LLM providers that use the `CHAT` or `CHAT_STREAMING` API route type.
	// +optional
	PromptEnrichment *AIPromptEnrichment `json:"prompt,omitempty"`

	// promptGuard enables adding guardrails to LLM requests and responses.
	// +optional
	PromptGuard *AIPromptGuard `json:"promptGuard,omitempty"`

	// Provide defaults to merge with user input fields. If the field is already set, the field in the request is used.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=64
	// +optional
	Defaults []FieldDefault `json:"defaults,omitempty"`
	// Provide overrides to merge with user input fields. If the field is already set, the field will be overwritten.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=64
	// +optional
	Overrides []FieldDefault `json:"overrides,omitempty"`

	// ModelAliases maps friendly model names to actual provider model names.
	// Example: {"fast": "gpt-3.5-turbo", "smart": "gpt-4-turbo"}
	// Note: This field is only applicable when using the agentgateway data plane.
	// +kubebuilder:validation:MaxProperties=64
	// +optional
	ModelAliases map[string]string `json:"modelAliases,omitempty"`

	// promptCaching enables automatic prompt caching for supported providers (AWS Bedrock).
	// Reduces API costs by caching static content like system prompts and tool definitions.
	// Only applicable for Bedrock Claude 3+ and Nova models.
	// +optional
	PromptCaching *PromptCachingConfig `json:"promptCaching,omitempty"`

	// routes defines how to identify the type of traffic to handle.
	// The keys are URL path suffixes matched using ends-with comparison (e.g., "/v1/chat/completions").
	// The special "*" wildcard matches any path.
	// If not specified, all traffic defaults to "completions" type.
	// +optional
	Routes map[string]RouteType `json:"routes,omitempty"`
}

// RouteType specifies how the AI gateway should process incoming requests
// based on the URL path and the API format expected.
// +kubebuilder:validation:Enum=completions;messages;models;passthrough;responses;anthropic_token_count
type RouteType string

const (
	// RouteTypeCompletions processes OpenAI /v1/chat/completions format requests
	RouteTypeCompletions RouteType = "completions"

	// RouteTypeMessages processes Anthropic /v1/messages format requests
	RouteTypeMessages RouteType = "messages"

	// RouteTypeModels handles /v1/models endpoint (returns available models)
	RouteTypeModels RouteType = "models"

	// RouteTypePassthrough sends requests to upstream as-is without LLM processing
	RouteTypePassthrough RouteType = "passthrough"

	// RouteTypeResponses processes OpenAI /v1/responses format requests
	RouteTypeResponses RouteType = "responses"

	// RouteTypeAnthropicTokenCount processes Anthropic /v1/messages/count_tokens format requests
	RouteTypeAnthropicTokenCount RouteType = "anthropic_token_count" //nolint:gosec // G101: False positive - this is a route type name, not credentials
)

// +kubebuilder:validation:AtLeastOneOf=authorization
type BackendMCP struct {
	// authorization defines MCPBackend level authorization. Unlike authorization at the HTTP level, which will reject
	// unauthorized requests with a 403 error, this policy works at the MCPBackend level.
	//
	// List operations, such as list_tools, will have each item evaluated. Items that do not meet the rule will be filtered.
	//
	// Get or call operations, such as call_tool, will evaluate the specific item and reject requests that do not meet the rule.
	// +optional
	Authorization *Authorization `json:"authorization,omitempty"`
	// authentication defines MCPBackend specific authentication rules.
	// TODO: this is problematic sort of. In agentgateway local mode, this setting is on route and backend, but we have
	// some hiding of this to make it set once but apply both.
	//Authentication *MCPAuthentication `json:"authentication,omitempty"`
}

type MCPAuthentication struct {
	// TODO: implement
}

// TODO: implement
type BackendHTTP struct {
	// poolIdleTimeout sets the timeout for idle sockets to be kept-alive for re-use in the connection pool.
	// +optional
	PoolIdleTimeout *metav1.Duration `json:"poolIdleTimeout,omitempty"`

	// http2WindowSize indicates the initial window size for stream-level flow control / for received data.
	// +kubebuilder:validation:Minimum=1
	// +optional
	HTTP2WindowSize *int32 `json:"http2WindowSize,omitempty"`
	// http2ConnectionWindowSize indicates the initial window size for connection-level flow control / for received data.
	// +kubebuilder:validation:Minimum=1
	// +optional
	HTTP2ConnectionWindowSize *int32 `json:"http2ConnectionWindowSize,omitempty"`
	// http2FrameSize sets the maxmimum frame size to use.
	// If unset, this defaults to 16kb
	// +kubebuilder:validation:Minimum=16384
	// +kubebuilder:validation:Maximum=1677215
	// +optional
	HTTP2FrameSize *int32 `json:"http2FrameSize,omitempty"`
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1s')",message="http2KeepaliveInterval must be at least 1 second"
	// +optional
	HTTP2KeepaliveInterval *metav1.Duration `json:"http2KeepaliveInterval,omitempty"`
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1s')",message="http2KeepaliveTimeout must be at least 1 second"
	// +optional
	HTTP2KeepaliveTimeout *metav1.Duration `json:"http2KeepaliveTimeout,omitempty"`

	// version specifies the HTTP protocol version to use when connecting to the backend.
	// If not specified, the version is automatically determined:
	// * Service types can specify it with 'appProtocol' on the Service port.
	// * If traffic is identified as gRPC, HTTP2 is used.
	// * If the incoming traffic was plaintext HTTP, the original protocol will be used.
	// * If the incoming traffic was HTTPS, HTTP1 will be used. This is because most clients will
	//   transparently upgrade HTTPS traffic to HTTP2, even if the backend doesn't support it
	// +kubebuilder:validation:Enum=HTTP1;HTTP2
	// +optional
	Version *HTTPVersion `json:"version,omitempty"`
}

type HTTPVersion string

const (
	HTTPVersion1 HTTPVersion = "HTTP1"
	HTTPVersion2 HTTPVersion = "HTTP2"
)

type BackendTCP struct {
	// keepAlive defines settings for enabling TCP keepalives on the connection.
	// +optional
	Keepalive *AgentgatewayKeepalive `json:"keepalive,omitempty"`
	// connectTimeout defines the deadline for establishing a connection to the destination.
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('100ms')",message="connectTimeout must be at least 100ms"
	// +optional
	ConnectTimeout *metav1.Duration `json:"connectTimeout,omitempty"`
}

// +kubebuilder:validation:AtLeastOneOf=request;response
type AgentTransformationPolicy struct {
	// request is used to modify the request path.
	// +optional
	Request *AgentTransform `json:"request,omitempty"`

	// response is used to modify the response path.
	// +optional
	Response *AgentTransform `json:"response,omitempty"`
}

// +kubebuilder:validation:AtLeastOneOf=set;add;remove;body
type AgentTransform struct {
	// set is a list of headers and the value they should be set to.
	//
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +optional
	Set []AgentHeaderTransformation `json:"set,omitempty"`

	// add is a list of headers to add to the request and what that value should be set to. If there is already a header
	// with these values then append the value as an extra entry.
	//
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +optional
	Add []AgentHeaderTransformation `json:"add,omitempty"`

	// Remove is a list of header names to remove from the request/response.
	//
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +optional
	Remove []AgentHeaderName `json:"remove,omitempty"`

	// body controls manipulation of the HTTP body.
	// +optional
	Body *CELExpression `json:"body,omitempty"`
}

// AgentHeaderName is the name of a header.
//
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=256
// +kubebuilder:validation:Pattern=`^:?[A-Za-z0-9!#$%&'*+\-.^_\x60|~]+$`
// +kubebuilder:validation:XValidation:rule="!self.startsWith(':') || self in [':authority', ':method', ':path', ':scheme', ':status']",message="pseudo-headers must be one of :authority, :method, :path, :scheme, or :status"
// +k8s:deepcopy-gen=false
type AgentHeaderName string

type AgentHeaderTransformation struct {
	// the name of the header to add.
	// +required
	Name AgentHeaderName `json:"name"`
	// value is the CEL expression to apply to generate the output value for the header.
	// +required
	Value CELExpression `json:"value"`
}

type AgentExtProcPolicy struct {
	// backendRef references the External Processor server to reach.
	// Supported types: Service and Backend.
	// +required
	BackendRef gwv1.BackendObjectReference `json:"backendRef"`
}

type AgentExtAuthPolicy struct {
	// backendRef references the External Authorization server to reach.
	//
	// Supported types: Service and Backend.
	// +required
	BackendRef gwv1.BackendObjectReference `json:"backendRef"`

	// forwardBody configures whether to include the HTTP body in the request. If enabled, the request body will be
	// buffered.
	// +optional
	ForwardBody *AgentExtAuthBody `json:"forwardBody,omitempty"`

	// contextExtensions specifies additional arbitrary key-value pairs to send to the authorization server.
	// +kubebuilder:validation:MaxProperties=64
	// +optional
	ContextExtensions map[string]string `json:"contextExtensions,omitempty"`
}

type AgentExtAuthBody struct {
	// maxSize specifies how large in bytes the largest body that will be buffered and sent to the authorization server. If
	// the body size is larger than maxSize, then the request will be rejected with a response.
	//
	// +kubebuilder:validation:Minimum=1
	// +required
	MaxSize int32 `json:"maxSize"`
}

// +kubebuilder:validation:AtLeastOneOf=local;global
type AgentRateLimit struct {
	// Local defines a local rate limiting policy.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +optional
	Local []AgentLocalRateLimitPolicy `json:"local,omitempty"`

	// Global defines a global rate limiting policy using an external service.
	// +optional
	Global *AgentRateLimitPolicy `json:"global,omitempty"`
}

type AgentRateLimitPolicy struct {
	// backendRef references the Rate Limit server to reach.
	// Supported types: Service and Backend.
	// +required
	BackendRef gwv1.BackendObjectReference `json:"backendRef"`

	// domain specifies the domain under which this limit should apply.
	// This is an arbitrary string that enables a rate limit server to distinguish between different applications.
	// +required
	Domain ShortString `json:"domain"`

	// Descriptors define the dimensions for rate limiting. These values are passed to the rate limit service which applies
	// configured limits based on them. Each descriptor represents a single rate limit rule with one or more entries.
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +required
	Descriptors []AgentRateLimitDescriptor `json:"descriptors"`
}

type RateLimitUnit string

const (
	RateLimitUnitTokens   RateLimitUnit = "Tokens"
	RateLimitUnitRequests RateLimitUnit = "Requests"
)

type AgentRateLimitDescriptor struct {
	// entries are the individual components that make up this descriptor.
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +required
	Entries []AgentRateLimitDescriptorEntry `json:"entries"`
	// unit defines what to use as the cost function. If unspecified, Requests is used.
	// +kubebuilder:validation:Enum=Requests;Tokens
	// +optional
	Unit *RateLimitUnit `json:"unit,omitempty"`
}

// AgentRateLimitDescriptorEntry defines a single entry in a rate limit descriptor.
type AgentRateLimitDescriptorEntry struct {
	// name specifies the name of the descriptor.
	// +required
	Name TinyString `json:"name"`
	// expression is a Common Expression Language (CEL) expression that defines the value for the descriptor.
	//
	// For example, to rate limit based on the Client IP: `source.address`.
	//
	// See https://agentgateway.dev/docs/reference/cel/ for more info.
	// +required
	Expression CELExpression `json:"expression"`
}

type LocalRateLimitUnit string

const (
	LocalRateLimitUnitSeconds LocalRateLimitUnit = "Seconds"
	LocalRateLimitUnitMinutes LocalRateLimitUnit = "Minutes"
	LocalRateLimitUnitHours   LocalRateLimitUnit = "Hours"
)

// AgentLocalRateLimitPolicy represents a policy for local rate limiting.
// It defines the configuration for rate limiting using a token bucket mechanism.
// +kubebuilder:validation:ExactlyOneOf=requests;tokens
type AgentLocalRateLimitPolicy struct {
	// requests specifies the number of HTTP requests per unit of time that are allowed. Requests exceeding this limit will fail with
	// a 429 error.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Requests *int32 `json:"requests,omitempty"`

	// tokens specifies the number of LLM tokens per unit of time that are allowed. Requests exceeding this limit will fail
	// with a 429 error.
	//
	// Both input and output tokens are counted. However, token counts are not known until the request completes. As a
	// result, token-based rate limits will apply to future requests only.
	//
	// +kubebuilder:validation:Minimum=1
	// +optional
	Tokens *int32 `json:"tokens,omitempty"`

	// unit specifies the unit of time that requests are limited based on.
	//
	// +kubebuilder:validation:Enum=Seconds;Minutes;Hours
	// +required
	Unit LocalRateLimitUnit `json:"unit"`

	// burst specifies an allowance of requests above the request-per-unit that should be allowed within a short period of time.
	// +optional
	Burst *int32 `json:"burst,omitempty"`
}

type AgentCorsPolicy struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	*gwv1.HTTPCORSFilter `json:",inline"`
}

type AgentCSRFPolicy struct {
	// additionalOrigin specifies additional source origins that will be allowed in addition to the destination origin. The
	// `Origin` consists of a scheme and a host, with an optional port, and takes the form `<scheme>://<host>(:<port>)`.
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +optional
	AdditionalOrigins []ShortString `json:"additionalOrigins,omitempty"`
}

type AgentHostnameRewriteConfig struct {
	// mode sets the hostname rewrite mode.
	//
	// The following may be specified:
	// * Auto: automatically set the Host header based on the destination.
	// * None: do not rewrite the Host header. The original Host header will be passed through.
	//
	// This setting defaults to Auto when connecting to hostname-based Backend types, and None otherwise (for Service or
	// IP-based Backends).
	// +required
	Mode AgentHostnameRewrite `json:"mode"`
}

type AgentTimeouts struct {
	// request specifies a timeout for an individual request from the gateway to a backend. This covers the time from when
	// the request first starts being sent from the gateway to when the full response has been received from the backend.
	//
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('100ms')",message="request must be at least 1ms"
	// +optional
	Request *metav1.Duration `json:"request,omitempty"`
}

// Retry defines the retry policy
type AgentRetry struct {
	*gwv1.HTTPRouteRetry `json:",inline"`
}

// accessLogs specifies how per-request access logs are emitted.
type AgentAccessLog struct {
	// filter specifies a CEL expression that is used to filter logs. A log will only be emitted if the expression evaluates
	// to 'true'.
	// +optional
	Filter *CELExpression `json:"filter,omitempty"`
	// attributes specifies customizations to the key-value pairs that are logged
	// +optional
	Attributes *AgentLogTracingFields `json:"attributes,omitempty"`
}

// +kubebuilder:validation:AtLeastOneOf=remove;add
type AgentLogTracingFields struct {
	// remove lists the default fields that should be removed. For example, "http.method".
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	// +optional
	Remove []TinyString `json:"remove,omitempty"`
	// add specifies additional key-value pairs to be added to each entry.
	// The value is a CEL expression. If the CEL expression fails to evaluate, the pair will be excluded.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:maxItems=64
	// +optional
	Add []AgentAttributeAdd `json:"add,omitempty"`
}

type AgentAttributeAdd struct {
	// +required
	Name ShortString `json:"name"`
	// +required
	Expression CELExpression `json:"expression"`
}

type TracingProtocol string

const (
	TracingProtocolHttp TracingProtocol = "HTTP"
	TracingProtocolGrpc TracingProtocol = "GRPC"
)

type AgentTracing struct {
	// backendRef references the OTLP server to reach.
	// Supported types: Service and Backend.
	// +required
	BackendRef gwv1.BackendObjectReference `json:"backendRef"`
	// protocol specifies the OTLP protocol variant to use.
	// +kubebuilder:default=HTTP
	// +kubebuilder:validation:Enum=HTTP;GRPC
	// +optional
	Protocol TracingProtocol `json:"protocol,omitempty"`

	// attributes specifies customizations to the key-value pairs that are included in the trace
	// +optional
	Attributes *AgentLogTracingFields `json:"attributes,omitempty"`

	// randomSampling is an expression to determine the amount of random sampling. Random sampling will initiate a new
	// trace span if the incoming request does not have a trace initiated already. This should evaluate to a float between
	// 0.0-1.0, or a boolean (true/false) If unspecified, random sampling is disabled.
	// +optional
	RandomSampling *CELExpression `json:"randomSampling,omitempty"`
	// clientSampling is an expression to determine the amount of client sampling. Client sampling determines whether to
	// initiate a new trace span if the incoming request does have a trace already. This should evaluate to a float between
	// 0.0-1.0, or a boolean (true/false) If unspecified, client sampling is 100% enabled.
	// +optional
	ClientSampling *CELExpression `json:"clientSampling,omitempty"`
}
