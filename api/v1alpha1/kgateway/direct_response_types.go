package kgateway

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=directresponses,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=directresponses/status,verbs=get;update;patch

// +kubebuilder:printcolumn:name="Accepted",type=string,JSONPath=".status.ancestors[*].conditions[?(@.type=='Accepted')].status",description="Direct response acceptance status"
// +kubebuilder:printcolumn:name="Attached",type=string,JSONPath=".status.ancestors[*].conditions[?(@.type=='Attached')].status",description="Direct response attachment status"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp",description="The age of the direct response."

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
	BodyFormat *shared.BodyFormat `json:"bodyFormat,omitempty"`
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
