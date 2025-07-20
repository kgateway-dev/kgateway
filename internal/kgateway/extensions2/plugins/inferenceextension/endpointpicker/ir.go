package endpointpicker

import (
	"encoding/json"
	"maps"
	"time"

	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	infextv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

const (
	// grpcPort is the default port number for a gRPC service.
	grpcPort = 9002
)

// inferencePool defines the internal representation of an inferencePool resource.
type inferencePool struct {
	objMeta metav1.ObjectMeta
	// podSelector is a label selector to select Pods that are members of the InferencePool.
	podSelector map[string]string
	// targetPort is the port number that should be targeted for Pods selected by Selector.
	targetPort int32
	// configRef is a reference to the extension configuration. A configRef is typically implemented
	// as a Kubernetes Service resource.
	configRef *service
	// errors is a list of errors that occurred while processing the InferencePool.
	errors []error
}

// newInferencePool returns the internal representation of the given pool.
func newInferencePool(pool *infextv1a2.InferencePool) *inferencePool {
	port := servicePort{name: "grpc", portNum: (int32(grpcPort))}
	if pool.Spec.ExtensionRef.PortNumber != nil {
		port.portNum = int32(*pool.Spec.ExtensionRef.PortNumber)
	}

	svcIR := &service{
		ObjectSource: ir.ObjectSource{
			Group:     infextv1a2.GroupVersion.Group,
			Kind:      wellknown.InferencePoolKind,
			Namespace: pool.Namespace,
			Name:      string(pool.Spec.ExtensionRef.Name),
		},
		obj:   pool,
		ports: []servicePort{port},
	}

	return &inferencePool{
		objMeta:     pool.ObjectMeta,
		podSelector: convertSelector(pool.Spec.Selector),
		targetPort:  int32(pool.Spec.TargetPortNumber),
		configRef:   svcIR,
	}
}

// In case multiple pools attached to the same resource, we sort by creation time.
func (ir *inferencePool) CreationTime() time.Time {
	return ir.objMeta.CreationTimestamp.Time
}

func (ir *inferencePool) Selector() map[string]string {
	if ir.podSelector == nil {
		return nil
	}
	return ir.podSelector
}

func (ir *inferencePool) Equals(other any) bool {
	otherPool, ok := other.(*inferencePool)
	if !ok {
		return false
	}
	return maps.EqualFunc(ir.Selector(), otherPool.Selector(), func(a, b string) bool {
		return a == b
	})
}

func convertSelector(selector map[infextv1a2.LabelKey]infextv1a2.LabelValue) map[string]string {
	result := make(map[string]string, len(selector))
	for k, v := range selector {
		result[string(k)] = string(v)
	}
	return result
}

// service defines the internal representation of a Service resource.
type service struct {
	// ObjectSource is a reference to the source object. Sometimes the group and kind are not
	// populated from api-server, so set them explicitly here, and pass this around as the reference.
	ir.ObjectSource `json:",inline"`

	// obj is the original object. Opaque to us other than metadata.
	obj metav1.Object

	// ports is a list of ports exposed by the service.
	ports []servicePort
}

// servicePort is an exposed post of a service.
type servicePort struct {
	// name is the name of the port.
	name string
	// portNum is the port number used to expose the service port.
	portNum int32
}

func (s service) ResourceName() string {
	return s.ObjectSource.ResourceName()
}

func (s service) Equals(in service) bool {
	return s.ObjectSource.Equals(in.ObjectSource) && versionEquals(s.obj, in.obj)
}

var _ krt.ResourceNamer = service{}
var _ krt.Equaler[service] = service{}
var _ json.Marshaler = service{}

func (s service) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Group     string
		Kind      string
		Name      string
		Namespace string
		Ports     []servicePort
	}{
		Group:     s.Group,
		Kind:      s.Kind,
		Namespace: s.Namespace,
		Name:      s.Name,
		Ports:     s.ports,
	})
}

func versionEquals(a, b metav1.Object) bool {
	var versionEquals bool
	if a.GetGeneration() != 0 && b.GetGeneration() != 0 {
		versionEquals = a.GetGeneration() == b.GetGeneration()
	} else {
		versionEquals = a.GetResourceVersion() == b.GetResourceVersion()
	}
	return versionEquals && a.GetUID() == b.GetUID()
}
