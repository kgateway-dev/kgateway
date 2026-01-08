package strategicpatch

import (
	"encoding/json"
	"fmt"
	"maps"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/agentgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// OverlayApplier applies AgentgatewayParameters overlays to rendered k8s objects
// using strategic merge patch semantics.
type OverlayApplier struct {
	params *agentgateway.AgentgatewayParameters
}

// NewOverlayApplier creates a new OverlayApplier with the given parameters.
func NewOverlayApplier(params *agentgateway.AgentgatewayParameters) *OverlayApplier {
	return &OverlayApplier{params: params}
}

// ApplyOverlays applies the overlays from AgentgatewayParameters to the rendered objects.
// It modifies the objects in place.
func (a *OverlayApplier) ApplyOverlays(objs []client.Object) error {
	if a.params == nil {
		return nil
	}

	overlays := a.params.Spec.AgentgatewayParametersOverlays

	for i, obj := range objs {
		gvk := obj.GetObjectKind().GroupVersionKind()
		var overlay *agentgateway.KubernetesResourceOverlay

		switch gvk.Kind {
		case wellknown.DeploymentGVK.Kind:
			overlay = overlays.Deployment
		case wellknown.ServiceGVK.Kind:
			overlay = overlays.Service
		case wellknown.ServiceAccountGVK.Kind:
			overlay = overlays.ServiceAccount
		default:
			continue
		}

		if overlay == nil {
			continue
		}

		patched, err := applyOverlay(obj, overlay, gvk)
		if err != nil {
			return fmt.Errorf("failed to apply overlay to %s/%s: %w", gvk.Kind, obj.GetName(), err)
		}
		objs[i] = patched
	}

	return nil
}

// applyOverlay applies a KubernetesResourceOverlay to a single object.
func applyOverlay(obj client.Object, overlay *agentgateway.KubernetesResourceOverlay, gvk schema.GroupVersionKind) (client.Object, error) {
	// Apply metadata first
	if overlay.Metadata != nil {
		if overlay.Metadata.Labels != nil {
			existingLabels := obj.GetLabels()
			if existingLabels == nil {
				existingLabels = make(map[string]string)
			}
			maps.Copy(existingLabels, overlay.Metadata.Labels)
			obj.SetLabels(existingLabels)
		}
		if overlay.Metadata.Annotations != nil {
			existingAnnotations := obj.GetAnnotations()
			if existingAnnotations == nil {
				existingAnnotations = make(map[string]string)
			}
			maps.Copy(existingAnnotations, overlay.Metadata.Annotations)
			obj.SetAnnotations(existingAnnotations)
		}
	}

	// Apply spec overlay using strategic merge patch if present
	if overlay.Spec != nil && len(overlay.Spec.Raw) > 0 {
		return applySpecOverlay(obj, overlay.Spec.Raw, gvk)
	}

	return obj, nil
}

// applySpecOverlay applies a spec overlay using strategic merge patch semantics.
func applySpecOverlay(obj client.Object, patchBytes []byte, gvk schema.GroupVersionKind) (client.Object, error) {
	// Get the schema for strategic merge patch
	dataObj, err := getDataObjectForGVK(gvk)
	if err != nil {
		return nil, fmt.Errorf("unsupported kind %s for strategic merge patch: %w", gvk.Kind, err)
	}

	// Serialize the original object to JSON
	originalBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal original object: %w", err)
	}

	// The patch from the user is for the spec field, but strategic merge patch
	// expects the full object structure. Wrap the patch in a spec field.
	wrappedPatch := map[string]json.RawMessage{
		"spec": patchBytes,
	}
	wrappedPatchBytes, err := json.Marshal(wrappedPatch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal wrapped patch: %w", err)
	}

	// Apply strategic merge patch
	patchedBytes, err := strategicpatch.StrategicMergePatch(originalBytes, wrappedPatchBytes, dataObj)
	if err != nil {
		return nil, fmt.Errorf("failed to apply strategic merge patch: %w", err)
	}

	// Deserialize back to the object
	patchedObj, err := deserializeToObject(patchedBytes, gvk)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize patched object: %w", err)
	}

	return patchedObj, nil
}

// getDataObjectForGVK returns an empty object of the appropriate type for strategic merge patch.
func getDataObjectForGVK(gvk schema.GroupVersionKind) (runtime.Object, error) {
	switch gvk.Kind {
	case wellknown.DeploymentGVK.Kind:
		return &appsv1.Deployment{}, nil
	case wellknown.ServiceGVK.Kind:
		return &corev1.Service{}, nil
	case wellknown.ServiceAccountGVK.Kind:
		return &corev1.ServiceAccount{}, nil
	default:
		return nil, fmt.Errorf("unsupported kind: %s", gvk.Kind)
	}
}

// deserializeToObject deserializes JSON bytes to a typed k8s object.
func deserializeToObject(data []byte, gvk schema.GroupVersionKind) (client.Object, error) {
	obj, err := getDataObjectForGVK(gvk)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, obj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patched object: %w", err)
	}

	// Ensure the GVK is set on the returned object
	clientObj := obj.(client.Object)
	clientObj.GetObjectKind().SetGroupVersionKind(gvk)

	return clientObj, nil
}
