package kgateway

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// RetryOnCondition specifies the condition under which retry takes place.
//
// +kubebuilder:validation:Enum={"5xx",gateway-error,reset,reset-before-request,connect-failure,envoy-ratelimited,retriable-4xx,refused-stream,retriable-status-codes,http3-post-connect-failure,cancelled,deadline-exceeded,internal,resource-exhausted,unavailable}
type RetryOnCondition string

// Retry defines the retry policy
//
// +kubebuilder:validation:XValidation:rule="has(self.retryOn) || has(self.statusCodes)",message="retryOn or statusCodes must be set."
// +kubebuilder:validation:XValidation:rule="!has(self.rateLimitedBackOff) || (has(self.retryOn) && self.retryOn.exists(r, r == 'envoy-ratelimited'))",message="rateLimitedBackOff requires retryOn to include envoy-ratelimited"
type Retry struct {
	// RetryOn specifies the conditions under which a retry should be attempted.
	// +optional
	//
	// +kubebuilder:validation:MinItems=1
	RetryOn []RetryOnCondition `json:"retryOn,omitempty"`

	// Attempts specifies the number of retry attempts for a request.
	// Defaults to 1 attempt if not set.
	// A value of 0 effectively disables retries.
	// +optional
	//
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	Attempts int32 `json:"attempts,omitempty"` //nolint:kubeapilinter

	// PerTryTimeout specifies the timeout per retry attempt (including the initial attempt).
	// If a global timeout is configured on a route, this timeout must be less than the global
	// route timeout.
	// It is specified as a sequence of decimal numbers, each with optional fraction and a unit suffix, such as "1s" or "500ms".
	// +optional
	//
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1ms')",message="retry.perTryTimeout must be at least 1ms."
	PerTryTimeout *metav1.Duration `json:"perTryTimeout,omitempty"`

	// StatusCodes specifies the HTTP status codes in the range 400-599 that should be retried in addition
	// to the conditions specified in RetryOn.
	// +optional
	//
	// +kubebuilder:validation:MinItems=1
	StatusCodes []gwv1.HTTPRouteRetryStatusCode `json:"statusCodes,omitempty"`

	// BackoffBaseInterval specifies the base interval used with a fully jittered exponential back-off between retries.
	// Defaults to 25ms if not set.
	// Given a backoff base interval B and retry number N, the back-off for the retry is in the range [0, (2^N-1)*B].
	// The backoff interval is capped at a max of 10 times the base interval.
	// E.g., given a value of 25ms, the first retry will be delayed randomly by 0-24ms, the 2nd by 0-74ms,
	// the 3rd by 0-174ms, and so on, and capped to a max of 10 times the base interval (250ms).
	// +optional
	//
	// +kubebuilder:default="25ms"
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1ms')",message="retry.backoffBaseInterval must be at least 1ms."
	BackoffBaseInterval *metav1.Duration `json:"backoffBaseInterval,omitempty"`

	// RateLimitedBackOff configures back-off behavior when a retry is triggered by the
	// envoy-ratelimited retry condition, distinct from the standard exponential back-off
	// configured via BackoffBaseInterval.
	// See the envoy docs for envoy.config.route.v3.RetryPolicy.RateLimitedRetryBackOff.
	// +optional
	RateLimitedBackOff *RateLimitedRetryBackOff `json:"rateLimitedBackOff,omitempty"`
}

// RateLimitedRetryBackOff configures the back-off interval envoy honors when a retry is
// triggered by the envoy-ratelimited retry condition. Envoy tries each of ResetHeaders in
// order and uses the first one that is present on the response and parses successfully.
// If none of the reset headers are present or parse successfully, the standard back-off
// strategy configured via BackoffBaseInterval is used instead.
type RateLimitedRetryBackOff struct {
	// ResetHeaders are response headers, such as "Retry-After" or "X-RateLimit-Reset", consulted
	// in order to compute the back-off interval. The first header present on the response that
	// parses successfully according to its Format is used; remaining headers are ignored.
	// Required: Envoy's RateLimitedRetryBackOff message requires at least one reset header
	// whenever it is present, so this cannot be left empty.
	// +required
	//
	// +kubebuilder:validation:MinItems=1
	ResetHeaders []ResetHeader `json:"resetHeaders"`

	// MaxInterval caps the back-off interval envoy will honor from a reset header.
	// If a header specifies a longer interval, it is discarded and the next header, if any, is tried.
	// Defaults to 300s if not set.
	// It is specified as a sequence of decimal numbers, each with optional fraction and a unit suffix, such as "1s" or "500ms".
	// +optional
	//
	// +kubebuilder:default="300s"
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1ms')",message="retry.rateLimitedBackOff.maxInterval must be at least 1ms."
	MaxInterval *metav1.Duration `json:"maxInterval,omitempty"`
}

// ResetHeaderFormat specifies how to parse a reset header's value into a back-off duration.
//
// +kubebuilder:validation:Enum=Seconds;UnixTimestamp
type ResetHeaderFormat string

const (
	// ResetHeaderFormatSeconds interprets the header value as the number of seconds to wait before retrying.
	ResetHeaderFormatSeconds ResetHeaderFormat = "Seconds"
	// ResetHeaderFormatUnixTimestamp interprets the header value as a Unix timestamp (seconds since
	// the epoch) at which the back-off period ends.
	ResetHeaderFormatUnixTimestamp ResetHeaderFormat = "UnixTimestamp"
)

// ResetHeader identifies a single response header consulted when computing a rate-limited
// retry back-off interval.
type ResetHeader struct {
	// Name is the response header to consult, e.g. "Retry-After" or "X-RateLimit-Reset".
	// +required
	Name gwv1.HTTPHeaderName `json:"name"`

	// Format specifies how to interpret the header's value.
	// +required
	Format ResetHeaderFormat `json:"format"`
}
