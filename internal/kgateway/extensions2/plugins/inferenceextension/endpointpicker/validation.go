package endpointpicker

import (
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	infextv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
)

// validatePool validates the given InferencePool and returns a list of errors if any validation checks fail.
func validatePool(pool *infextv1a2.InferencePool, svcCol krt.Collection[*corev1.Service]) []error {
	var errs []error

	if pool.Spec.ExtensionRef == nil {
		errs = append(errs, fmt.Errorf("no extensionRef is defined"))
		return errs
	}

	if pool.Spec.ExtensionRef.ExtensionReference.Group != nil && *pool.Spec.ExtensionRef.ExtensionReference.Group != "" {
		errs = append(errs, fmt.Errorf("invalid extensionRef: Group %s is not supported", *pool.Spec.ExtensionRef.ExtensionReference.Group))
	}

	if pool.Spec.ExtensionRef.ExtensionReference.Kind != nil && *pool.Spec.ExtensionRef.ExtensionReference.Kind != "Service" {
		errs = append(errs, fmt.Errorf("invalid extensionRef: Kind %s is not supported", *pool.Spec.ExtensionRef.ExtensionReference.Group))
	}

	if pool.Spec.ExtensionRef.ExtensionReference.PortNumber != nil &&
		(*pool.Spec.ExtensionRef.ExtensionReference.PortNumber < infextv1a2.PortNumber(1) ||
			*pool.Spec.ExtensionRef.ExtensionReference.PortNumber > infextv1a2.PortNumber(65535)) {
		errs = append(errs, fmt.Errorf("invalid extensionRef: PortNumber %d is not supported", *pool.Spec.ExtensionRef.ExtensionReference.PortNumber))
	}

	// Check for the existence of the referenced service.
	svcName := pool.Spec.ExtensionRef.Name
	svcNN := types.NamespacedName{Namespace: pool.Namespace, Name: string(svcName)}
	svcObj := svcCol.GetKey(svcNN.String())
	if svcObj == nil {
		errs = append(errs, fmt.Errorf("invalid extensionRef: Service %s/%s not found", pool.Namespace, svcName))
	}

	return errs
}
