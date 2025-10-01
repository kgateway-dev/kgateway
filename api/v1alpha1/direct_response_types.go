package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DirectResponseSpec   `json:"spec,omitempty"`
	Status DirectResponseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type DirectResponseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DirectResponse `json:"items"`
}

// DirectResponseSpec describes the desired state of a DirectResponse.
type DirectResponseSpec struct {
	// StatusCode defines the HTTP status code to return for this route.
	//
	// +required
	// +kubebuilder:validation:Minimum=200
	// +kubebuilder:validation:Maximum=599
	StatusCode int32 `json:"status"`
	// Body defines the content to be returned in the HTTP response body.
	// The maximum length of the body is restricted to prevent excessively large responses.
	// If this field is omitted, no body is included in the response.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	Body *string `json:"body,omitempty"`
}

// DirectResponseStatus defines the observed state of a DirectResponse.
type DirectResponseStatus struct{}

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
