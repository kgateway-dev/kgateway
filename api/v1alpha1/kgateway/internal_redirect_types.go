package kgateway

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// InternalRedirectResponseCode is a 3xx response code supported for internal redirects.
//
// +kubebuilder:validation:Enum=301;302;303;307;308
type InternalRedirectResponseCode int32

// InternalRedirect configures the gateway to handle upstream 3xx redirects inside the
// gateway. The gateway follows a valid, fully qualified Location header and returns only
// the final response to the client.
// Applies only to routes that forward traffic to a backend.
type InternalRedirect struct {
	// RedirectResponseCodes are upstream status codes that trigger internal redirects.
	// If unset, only 302 redirects are followed.
	// +optional
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=5
	// +kubebuilder:validation:XValidation:rule="self.all(c, self.exists_one(c2, c2 == c))",message="redirectResponseCodes must not contain duplicates"
	RedirectResponseCodes []InternalRedirectResponseCode `json:"redirectResponseCodes,omitempty"`

	// AllowCrossSchemeRedirect permits redirects across http/https schemes.
	// Defaults to false.
	// +optional
	AllowCrossSchemeRedirect *bool `json:"allowCrossSchemeRedirect,omitempty"`

	// ResponseHeadersToCopy are copied from the redirect response to the
	// internally redirected request.
	// +optional
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:XValidation:rule="self.all(h, self.exists_one(h2, h2 == h))",message="responseHeadersToCopy must not contain duplicates"
	ResponseHeadersToCopy []gwv1.HTTPHeaderName `json:"responseHeadersToCopy,omitempty"`

	// MaxRedirects caps followed redirects for a single downstream request.
	// Defaults to 1.
	// +optional
	//
	// +kubebuilder:validation:Minimum=1
	MaxRedirects *uint32 `json:"maxRedirects,omitempty"`
}
