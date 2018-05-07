package v1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Upstream is the generic Kubernetes API object wrapper for Gloo Upstreams
type Upstream struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec *Spec        `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// UpstreamList is the generic Kubernetes API object wrapper
type UpstreamList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta  `json:"metadata"`
	Items []Upstream `json:"items"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualService is the generic Kubernetes API object wrapper for Gloo VirtualServices
type VirtualService struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec *Spec        `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualServiceList is the generic Kubernetes API object wrapper
type VirtualServiceList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta        `json:"metadata"`
	Items []VirtualService `json:"items"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Report is the generic Kubernetes API object wrapper for Gloo Reports
type Report struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec *Spec        `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ReportList is the generic Kubernetes API object wrapper
type ReportList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata"`
	Items []Report  `json:"items"`
}

// spec implements deepcopy
type Spec map[string]interface{}

func (in *Spec) DeepCopyInto(out *Spec) {
	if in == nil {
		out = nil
		return
	}
	data, err := json.Marshal(in)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(data, &out)
	if err != nil {
		panic(err)
	}
}
