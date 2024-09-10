package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

const (
	Group   = "gateway.gloo.solo.io"
	Version = "v1alpha1"

	GatewayParametersKind = "GatewayParameters"
)

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: Group, Version: Version}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme

	GatewayParametersGVK = schema.GroupVersionKind{
		Group:   Group,
		Version: Version,
		Kind:    GatewayParametersKind,
	}
)
