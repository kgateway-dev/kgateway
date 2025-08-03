package endpointpicker

import (
	"encoding/json"
	"fmt"
	"maps"
	"sync"
	"time"

	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	infextv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

const (
	// grpcPort is the default port number for a gRPC service.
	grpcPort = 9002
)

// inferencePool defines the internal representation of an inferencePool resource.
type inferencePool struct {
	// obj is the original object. Opaque to us other than metadata.
	obj metav1.Object
	// podSelector is a label selector to select Pods that are members of the InferencePool.
	podSelector map[string]string
	// targetPort is the port number that should be targeted for Pods selected by Selector.
	targetPort int32
	// configRef is a reference to the extension configuration. A configRef is typically implemented
	// as a Kubernetes Service resource.
	configRef *service
	// mu is a mutex to protect access to the errors list.
	mu sync.Mutex
	// errors is a list of errors that occurred while processing the InferencePool.
	errors []error
	// endpoints define the list of endpoints resolved by the podSelector.
	endpoints []endpoint
	// failOpen configures how the proxy handles traffic when the EPP extension is
	// non-responsive. When set to `false` and the gRPC stream cannot be established, or if
	// it is closed prematurely with an error, the request will fail. When set to `true` and
	// the gRPC stream cannot be established, the request is forwarded based on the cluster
	// load balancing configuration.
	//
	// Defaults to `false`.
	//
	failOpen bool
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
		obj:         pool,
		podSelector: convertSelector(pool.Spec.Selector),
		targetPort:  int32(pool.Spec.TargetPortNumber),
		configRef:   svcIR,
		endpoints:   []endpoint{},
		failOpen:    isFailOpen(pool),
	}
}

func (ir *inferencePool) setEndpoints(eps []endpoint) {
	ir.endpoints = eps
}

// resolvePoolEndpoints returns the slice of <IP:Port> for the given pool
// by looking up only the pods that index to it.
func (ir *inferencePool) resolvePoolEndpoints(
	idx krt.Index[string, krtcollections.LocalityPod],
) []endpoint {
	key := fmt.Sprintf("%s/%s", ir.obj.GetNamespace(), ir.obj.GetName())

	var eps []endpoint
	for _, p := range idx.Lookup(key) {
		if ip := p.Address(); ip != "" {
			eps = append(eps, endpoint{address: ip, port: ir.targetPort})
		}
	}

	return eps
}

// In case multiple pools attached to the same resource, we sort by creation time.
func (ir *inferencePool) CreationTime() time.Time {
	return ir.obj.GetCreationTimestamp().Time
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
	// Compare pod selector
	if !maps.Equal(ir.Selector(), otherPool.Selector()) {
		return false
	}
	// Compare error presence (we only need the boolean)
	if ir.hasErrors() != otherPool.hasErrors() {
		return false
	}
	// Compare endpoint set (order‑insensitive)
	if len(ir.endpoints) != len(otherPool.endpoints) {
		return false
	}
	seen := make(map[string]struct{}, len(ir.endpoints))
	for _, ep := range ir.endpoints {
		seen[ep.string()] = struct{}{}
	}
	for _, ep := range otherPool.endpoints {
		if _, ok := seen[ep.string()]; !ok {
			return false
		}
	}
	// Compare target port
	if ir.targetPort != otherPool.targetPort {
		return false
	}
	// Compare configRef
	if !ir.configRefEquals(otherPool) {
		return false
	}
	// Compare failure mode
	if !ir.failOpenEqual(otherPool) {
		return false
	}
	return true
}

// configRefEquals checks whether two pools refer to the same extension config service.
func (ir *inferencePool) configRefEquals(other *inferencePool) bool {
	if ir.configRef == nil && other.configRef == nil {
		return true
	}
	if (ir.configRef == nil) != (other.configRef == nil) {
		return false
	}
	return ir.configRef.Equals(*other.configRef)
}

// setErrors atomically replaces p.errors under lock.
func (ir *inferencePool) setErrors(errs []error) {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	ir.errors = errs
}

// snapshotErrors returns a copy of p.errors under lock.
func (ir *inferencePool) snapshotErrors() []error {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	out := make([]error, len(ir.errors))
	copy(out, ir.errors)
	return out
}

// hasErrors checks if the inferencePool has any errors.
func (ir *inferencePool) hasErrors() bool {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	return len(ir.errors) > 0
}

func (ir *inferencePool) failOpenEqual(other *inferencePool) bool {
	return ir.failOpen == other.failOpen
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

// endpoint defines the internal representation of an endpoint.
type endpoint struct {
	// address is the IP address address of the endpoint.
	address string
	// port is the port exposed by the endpoint.
	port int32
}

func (e endpoint) string() string {
	return fmt.Sprintf("%s:%d", e.address, e.port)
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

func isFailOpen(pool *infextv1a2.InferencePool) bool {
	if pool == nil ||
		pool.Spec.EndpointPickerConfig.ExtensionRef == nil {
		return false
	}

	if pool.Spec.EndpointPickerConfig.ExtensionRef.ExtensionConnection.FailureMode == nil ||
		*pool.Spec.EndpointPickerConfig.ExtensionRef.ExtensionConnection.FailureMode == infextv1a2.FailClose {
		return false
	}

	return true
}
