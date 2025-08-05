package endpointpicker

import (
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	infextv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// defaultInfPoolExtRefPort is the default port number (grpc-ext-proc port) for an
// InferencePool EPP extension reference.
const defaultInfPoolExtRefPort = 9002

// validatePool verifies that the given InferencePool is valid.
func validatePool(pool *infextv1a2.InferencePool, svcCol krt.Collection[*corev1.Service]) []error {
	var errs []error

	// ExtensionRef must be defined
	ext := pool.Spec.ExtensionRef
	if ext == nil {
		return append(errs, fmt.Errorf("no extensionRef is defined"))
	}

	// Group must be empty (core API group only)
	if ext.Group != nil && *ext.Group != "" {
		errs = append(errs,
			fmt.Errorf("invalid extensionRef: only core API group supported, got %q", *ext.Group))
	}

	// Only Service kind is allowed
	kind := wellknown.ServiceKind
	if ext.Kind != nil {
		kind = string(*ext.Kind)
	}
	if kind != wellknown.ServiceKind {
		errs = append(errs,
			fmt.Errorf("invalid extensionRef: Kind %q is not supported (only Service)", kind))
	}

	// PortNumber defaults to 9002 and must be 1-65535 (rfc1340 port range)
	port := int32(defaultInfPoolExtRefPort)
	if ext.PortNumber != nil {
		port = int32(*ext.PortNumber)
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
		if sp.Port == port && proto == corev1.ProtocolTCP {
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
