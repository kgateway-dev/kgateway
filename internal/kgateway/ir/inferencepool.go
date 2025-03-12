package ir

import (
	"maps"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	infextv1a1 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// InferencePool defines the internal representation of an InferencePool resource.
type InferencePool struct {
	ObjMeta metav1.ObjectMeta
	// PodSelector is a label selector to select Pods that are members of the InferencePool.
	PodSelector map[string]string
	// TargetPort is the port number that should be targeted for Pods selected by Selector.
	TargetPort int32
	// ConfigRef is a reference to the extension configuration. A ConfigRef is typically implemented
	// as a Kubernetes Service resource.
	ConfigRef *Service
}

// NewInferencePool returns the internal representation of the given pool.
func NewInferencePool(pool *infextv1a1.InferencePool) *InferencePool {
	if pool == nil || pool.Spec.ExtensionRef == nil {
		return nil
	}

	port := ServicePort{Name: "grpc", PortNum: (int32(grpcPort))}
	if pool.Spec.ExtensionRef.TargetPortNumber != nil {
		port.PortNum = *pool.Spec.ExtensionRef.TargetPortNumber
	}

	svcIR := &Service{
		ObjectSource: ObjectSource{
			Group:     infextv1a1.GroupVersion.Group,
			Kind:      wellknown.InferencePoolKind,
			Namespace: pool.Namespace,
			Name:      pool.Spec.ExtensionRef.Name,
		},
		Obj:   pool,
		Ports: []ServicePort{port},
	}

	return &InferencePool{
		ObjMeta:     pool.ObjectMeta,
		PodSelector: convertSelector(pool.Spec.Selector),
		TargetPort:  pool.Spec.TargetPortNumber,
		ConfigRef:   svcIR,
	}
}

// In case multiple pools attached to the same resource, we sort by creation time.
func (ir *InferencePool) CreationTime() time.Time {
	return ir.ObjMeta.CreationTimestamp.Time
}

func (ir *InferencePool) Selector() map[string]string {
	if ir.PodSelector == nil {
		return nil
	}
	return ir.PodSelector
}

func (ir *InferencePool) Equals(other any) bool {
	otherPool, ok := other.(*InferencePool)
	if !ok {
		return false
	}
	return maps.EqualFunc(ir.Selector(), otherPool.Selector(), func(a, b string) bool {
		return a == b
	})
}

func convertSelector(selector map[infextv1a1.LabelKey]infextv1a1.LabelValue) map[string]string {
	result := make(map[string]string, len(selector))
	for k, v := range selector {
		result[string(k)] = string(v)
	}
	return result
}
