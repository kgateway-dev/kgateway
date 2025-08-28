package endpointpicker

import (
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// validatePool verifies that the given InferencePool is valid.
func validatePool(pool *inf.InferencePool, svcCol krt.Collection[*corev1.Service]) []error {
	var errs []error
	ext := pool.Spec.EndpointPickerRef

	// Group must be empty (core API group only)
	if ext.Group != nil && *ext.Group != "" {
		errs = append(errs,
			fmt.Errorf("invalid extensionRef: only core API group supported, got %q", *ext.Group))
	}

	// Only Service kind is allowed
	if ext.Kind != wellknown.ServiceKind {
		errs = append(errs,
			fmt.Errorf("invalid extensionRef: Kind %q is not supported (only Service)", wellknown.ServiceKind))
	}

	// Inferencepool v1 only supports a single target port
	if len(pool.Spec.TargetPorts) != 1 {
		errs = append(errs,
			fmt.Errorf("invalid InferencePool: must have exactly one target port"))
	}

	// PortNumber defaults to 9002 and must be 1-65535 (rfc1340 port range)
	port := inf.PortNumber(grpcPort)
	if ext.PortNumber != nil {
		port = *ext.PortNumber
	}
	if port < 1 || port > 65535 {
		errs = append(errs,
			fmt.Errorf("invalid extensionRef: PortNumber %d is out of range", port))
	}

	svcNN := types.NamespacedName{Namespace: pool.Namespace, Name: string(ext.Name)}
	svcPtr := svcCol.GetKey(svcNN.String())
	if svcPtr == nil {
		errs = append(errs,
			fmt.Errorf("invalid extensionRef: Service %s/%s not found",
				pool.Namespace, ext.Name))
		return errs
	}
	svc := *svcPtr

	// ExternalName Services are not allowed
	if svc.Spec.Type == corev1.ServiceTypeExternalName {
		errs = append(errs,
			fmt.Errorf("invalid extensionRef: must use any Service type other than ExternalName"))
	}

	// Service must expose the requested TCP port
	found := false
	for _, sp := range svc.Spec.Ports {
		proto := sp.Protocol
		if proto == "" {
			proto = corev1.ProtocolTCP // default
		}
		if sp.Port == int32(port) && proto == corev1.ProtocolTCP {
			found = true
			break
		}
	}
	if !found {
		errs = append(errs,
			fmt.Errorf("TCP port %d not found on Service %s/%s",
				port, pool.Namespace, ext.Name))
	}

	return errs
}
