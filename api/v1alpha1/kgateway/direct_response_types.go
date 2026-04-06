package kgateway

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=directresponses,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=directresponses/status,verbs=get;update;patch

// DirectResponse contains configuration for defining direct response routes.
//
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway
// +kubebuilder:subresource:status
type DirectResponse struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +required
	Spec DirectResponseSpec `json:"spec"`
	// +optional
	Status gwv1.PolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type DirectResponseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DirectResponse `json:"items"`
}

// DirectResponseSpec describes the desired state of a DirectResponse.
// +kubebuilder:validation:AtMostOneOf=body;bodyFormat
type DirectResponseSpec struct {
	// StatusCode defines the HTTP status code to return for this route.
	//
	// +required
	// +kubebuilder:validation:Minimum=200
	// +kubebuilder:validation:Maximum=599
	StatusCode int32 `json:"status"`
	// Body defines the content to be returned in the HTTP response body.
	// The maximum length of the body is restricted to prevent excessively large responses.
	// If this field and BodyFormat are both omitted, no body is included in the response.
	// Mutually exclusive with BodyFormat.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	Body *string `json:"body,omitempty"`
	// BodyFormat defines the content to be returned in the HTTP response body as an Envoy format string.
	// If this field and Body are both omitted, no body is included in the response.
	// Mutually exclusive with Body.
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-directresponseaction-body-format for details.
	//
	// +optional
	BodyFormat *BodyFormat `json:"bodyFormat,omitempty"`
}

// BodyFormat configures an Envoy response body using formatting. Either JSON or Text must be specified.
// +kubebuilder:validation:ExactlyOneOf=json;text
type BodyFormat struct {
	// ContentType defines the HTTP Content-Type header to be sent with the response.
	// By default, `text/plain` is used for the Text format and `application/json` for the JSON format.
	// Note: This setting does not currently take effect due to a bug in Envoy, a fix for which is pending release.
	// The option is included for completeness and will become effective with a future version of Envoy.
	// +optional
	ContentType *string `json:"contentType,omitempty"`
	// Text is a format string by which Envoy will format the response body.
	// Mutually exclusive with JSON.
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/substitution_format_string.proto#envoy-v3-api-field-config-core-v3-substitutionformatstring-text-format for details.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	Text *string `json:"text,omitempty"`
	// JSON is a format object by which Envoy will produce a JSON response body.
	// Mutually exclusive with Text.
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/substitution_format_string.proto#envoy-v3-api-field-config-core-v3-substitutionformatstring-json-format for details.
	//
	// Setting a field to `null` in the JSON object requires the use of
	// `kubectl apply --server-side` or equivalent. With the default client-side
	// `kubectl apply`, null values are stripped by kubectl before reaching
	// the API server.
	// +optional
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields
	JSON *apiextensionsv1.JSON `json:"json,omitempty"`
}

// GetStatus returns the HTTP status code to return for this route.
func (in *DirectResponse) GetStatusCode() int32 {
	if in == nil {
		return 0
	}
	return in.Spec.StatusCode
}

// GetBody returns the content to be returned in the HTTP response body.
func (in *DirectResponse) GetBody() *string {
	if in == nil {
		return nil
	}
	return in.Spec.Body
}
