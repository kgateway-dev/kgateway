package kuberesource

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// ConvertUnstructured converts an unstructured object to a typed object
// based on https://github.com/solo-io/k8s-utils/blob/41d59afd326dbffb6a50d7c96fe02def8e92e08b/installutils/kuberesource/unstructured.go#L137
// Add types to this as needed
func ConvertUnstructured(res *unstructured.Unstructured) (runtime.Object, error) {
	rawJson, err := res.MarshalJSON()
	if err != nil {
		return nil, err
	}

	var typeMeta metav1.TypeMeta
	if err := json.Unmarshal(rawJson, &typeMeta); err != nil {
		return nil, errors.Wrapf(err, "parsing raw yaml into &typeMeta as %+v", typeMeta)
	}

	kind := typeMeta.Kind

	var obj runtime.Object
	switch kind {
	case "GatewayParameters":
		obj = &v1alpha1.GatewayParameters{TypeMeta: typeMeta}
	default:
		return nil, errors.Errorf("cannot convert kind %v", kind)
	}

	if err := json.Unmarshal(rawJson, obj); err != nil {
		return nil, errors.Wrapf(err, "parsing raw yaml into &obj as %+v", obj)
	}
	return obj, nil

}
