package shared

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

	// PerTryTimeout specifies the timeout per retry attempt (incliding the initial attempt).
	// If a global timeout is configured on a route, this timeout must be less than the global
	// route timeout.
	// It is specified as a sequence of decimal numbers, each with optional fraction and a unit suffix, such as "1s" or "500ms".
	// +optional
	//
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
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1ms')",message="retry.backoffBaseInterval must be at least 1ms."
	BackoffBaseInterval *metav1.Duration `json:"backoffBaseInterval,omitempty"`
}
