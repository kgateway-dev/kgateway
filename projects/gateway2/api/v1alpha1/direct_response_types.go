package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DirectResponseRoute contains configuration for defining direct response routes.
//
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=gloo-gateway,app.kubernetes.io/name=gloo-gateway}
// +kubebuilder:resource:categories=gloo-gateway,shortName=drr
// +kubebuilder:subresource:status
type DirectResponseRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DirectResponseRouteSpec   `json:"spec,omitempty"`
	Status DirectResponseRouteStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type DirectResponseRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DirectResponseRoute `json:"items"`
}

// DirectResponseRouteSpec describes the desired state of a DirectResponseRoute.
type DirectResponseRouteSpec struct {
	// Status defines the HTTP status code to return for this route.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=200
	// +kubebuilder:validation:Maximum=599
	Status uint32 `json:"status"`
	// Body defines the content to be returned in the HTTP response body.
	// The maximum length of the body is restricted to prevent excessively large responses.
	//
	// +kubebuilder:validation:MaxLength=4096
	// +kubebuilder:validation:Optional
	Body string `json:"body,omitempty"`
}

// DirectResponseRouteStatus defines the observed state of a DirectResponseRoute.
type DirectResponseRouteStatus struct{}

// GetStatus returns the HTTP status code to return for this route.
func (in *DirectResponseRoute) GetStatus() uint32 {
	if in == nil {
		return 0
	}
	return in.Spec.Status
}

// GetBody returns the content to be returned in the HTTP response body.
func (in *DirectResponseRoute) GetBody() string {
	if in == nil {
		return ""
	}
	return in.Spec.Body
}

func init() {
	SchemeBuilder.Register(&DirectResponseRoute{}, &DirectResponseRouteList{})
}
