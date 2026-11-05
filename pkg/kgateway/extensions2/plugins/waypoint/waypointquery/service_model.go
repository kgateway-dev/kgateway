package waypointquery

import (
	"fmt"

	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils/backendaddress"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// Service is a common type to use between Service and ServiceEntry
type Service struct {
	client.Object
	GroupKind schema.GroupKind
	Aliases   []ir.ObjectSource
}

// Keys give all the name/namespace/group/kind keys
// including those from aliases.
// Specifically returns the aliases _first_.
func (s Service) Keys() []ir.ObjectSource {
	aliases := s.Aliases

	// Check if the first alias is a ServiceEntry's own ObjectSource
	if len(aliases) > 0 && aliases[0].Equals(ir.ObjectSource{
		Name:      s.GetName(),
		Namespace: s.GetNamespace(),
		Group:     wellknown.ServiceEntryGVK.Group,
		Kind:      wellknown.ServiceEntryGVK.Kind,
	}) {
		// ServiceEntry has self as the first one (see BuildServiceEntryBackendObjectIR).
		// We want to return the aliases _after_ the self.
		aliases = aliases[1:]
	}
	return append(aliases, ir.ObjectSource{
		Name:      s.GetName(),
		Namespace: s.GetNamespace(),
		Group:     s.GroupKind.Group,
		Kind:      s.GroupKind.Kind,
	})
}

func (s Service) Kind() string {
	return s.GroupKind.Kind
}

func (s Service) String() string {
	return serviceKey(s.Kind(), s.GetNamespace(), s.GetName())
}

func FromService(svc *corev1.Service) Service {
	return Service{
		Object:    svc,
		GroupKind: wellknown.ServiceGVK.GroupKind(),
	}
}

func FromServiceEntry(se *networkingclient.ServiceEntry, aliases []ir.ObjectSource) Service {
	return Service{
		Object:    se,
		GroupKind: wellknown.ServiceEntryGVK.GroupKind(),
		Aliases:   aliases,
	}
}

func BackendAddresses(ir ir.BackendObjectIR) []string {
	var addresses []string
	switch ir.Obj.(type) {
	case *corev1.Service:
		addresses = backendaddress.ServiceAddresses(ir.Obj.(*corev1.Service))
	case *networkingclient.ServiceEntry:
		addresses = backendaddress.ServiceEntryAddresses(ir.Obj.(*networkingclient.ServiceEntry))
	}
	return addresses
}

func serviceKey(kind, namespace, name string) string {
	return fmt.Sprintf("%s(%s/%s)", kind, namespace, name)
}

func ServiceKeyFromObject(obj metav1.Object) string {
	var kind string
	switch obj.(type) {
	case *corev1.Service:
		kind = wellknown.ServiceGVK.GroupKind().Kind
	case *networkingclient.ServiceEntry:
		kind = wellknown.ServiceEntryGVK.GroupKind().Kind
	default:
		return ""
	}
	return serviceKey(kind, obj.GetNamespace(), obj.GetName())
}
