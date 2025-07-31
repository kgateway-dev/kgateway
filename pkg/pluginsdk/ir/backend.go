package ir

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	pluginsdkreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

type ObjectSource struct {
	Group     string `json:"group,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

// GetKind returns the kind of the route.
func (c ObjectSource) GetGroupKind() schema.GroupKind {
	return schema.GroupKind{
		Group: c.Group,
		Kind:  c.Kind,
	}
}

// GetName returns the name of the route.
func (c ObjectSource) GetName() string {
	return c.Name
}

// GetNamespace returns the namespace of the route.
func (c ObjectSource) GetNamespace() string {
	return c.Namespace
}

func (c ObjectSource) ResourceName() string {
	return fmt.Sprintf("%s/%s/%s/%s", c.Group, c.Kind, c.Namespace, c.Name)
}

func (c ObjectSource) String() string {
	return fmt.Sprintf("%s/%s/%s/%s", c.Group, c.Kind, c.Namespace, c.Name)
}

func (c ObjectSource) Equals(in ObjectSource) bool {
	return c.Namespace == in.Namespace && c.Name == in.Name && c.Group == in.Group && c.Kind == in.Kind
}

type Namespaced interface {
	GetName() string
	GetNamespace() string
}

type AppProtocol string

const (
	DefaultAppProtocol   AppProtocol = ""
	HTTP2AppProtocol     AppProtocol = "http2"
	WebSocketAppProtocol AppProtocol = "ws"
)

// ParseAppProtocol takes an app protocol string provided on a Backend or Kubernetes Service, and maps it
// to one of the app protocol types supported by kgateway (http2, websocket, or default).
// Recognizes http2 app protocols defined by istio (https://istio.io/latest/docs/ops/configuration/traffic-management/protocol-selection/)
// and GEP-1911 (https://gateway-api.sigs.k8s.io/geps/gep-1911/#api-semantics).
func ParseAppProtocol(appProtocol *string) AppProtocol {
	switch ptr.Deref(appProtocol, "") {
	case string(v1alpha1.AppProtocolHttp2):
		fallthrough
	case string(v1alpha1.AppProtocolGrpc):
		fallthrough
	case string(v1alpha1.AppProtocolGrpcWeb):
		fallthrough
	case string(v1alpha1.AppProtocolKubernetesH2C):
		return HTTP2AppProtocol
	case string(v1alpha1.AppProtocolKubernetesWs):
		return WebSocketAppProtocol
	default:
		return DefaultAppProtocol
	}
}

type BackendObjectIR struct {
	// Ref to source object. sometimes the group and kind are not populated from api-server, so
	// set them explicitly here, and pass this around as the reference.
	ObjectSource `json:",inline"`
	// optional port for if ObjectSource is a service that can have multiple ports.
	Port int32
	// optional application protocol for the backend. Can be used to enable http2.
	AppProtocol AppProtocol

	// prefix the cluster name with this string to distinguish it from other GVKs.
	// here explicitly as it shows up in stats. each (group, kind) pair should have a unique prefix.
	GvPrefix string
	// for things that integrate with destination rule, we need to know what hostname to use.
	CanonicalHostname string
	// original object. Opaque to us other than metadata.
	Obj metav1.Object

	// can this just be any?
	// i think so, assuming obj -> objir is a 1:1 mapping.
	ObjIr interface{ Equals(any) bool }

	// Aliases that we can key by when referencing this backend from policy or routes.
	Aliases []ObjectSource

	// ExtraKey allows ensuring uniqueness in the KRT key
	// when there is more than one backend per ObjectSource+port.
	// TODO this is a hack for ServiceEntry to workaround only having one
	// CanonicalHostname. We should see if it's possible to have multiple
	// CanonicalHostnames.
	ExtraKey string

	AttachedPolicies AttachedPolicies

	// Errors is a list of errors, if any, encountered while constructing this BackendObject
	// Not added to Equals() as it is derived from the inner ObjIr, which is already evaluated
	Errors []error

	// Name is the pre-calculated resource name. used as the krt resource name.
	resourceName string

	// TrafficDistribution is the desired traffic distribution for the backend.
	// Default is any (no priority).
	TrafficDistribution wellknown.TrafficDistribution
}

// NewBackendObjectIR creates a new BackendObjectIR with pre-calculated resource name
func NewBackendObjectIR(objSource ObjectSource, port int32, extraKey string) BackendObjectIR {
	return BackendObjectIR{
		ObjectSource: objSource,
		Port:         port,
		ExtraKey:     extraKey,
		resourceName: BackendResourceName(objSource, port, extraKey),
	}
}

func (c BackendObjectIR) ResourceName() string {
	return c.resourceName
}

func BackendResourceName(objSource ObjectSource, port int32, extraKey string) string {
	var sb strings.Builder
	sb.WriteString(objSource.ResourceName())
	sb.WriteString(fmt.Sprintf(":%d", port))

	if extraKey != "" {
		sb.WriteRune('_')
		sb.WriteString(extraKey)
	}
	return sb.String()
}

func (c BackendObjectIR) Equals(in BackendObjectIR) bool {
	objEq := c.ObjectSource.Equals(in.ObjectSource)
	objVersionEq := versionEquals(c.Obj, in.Obj)
	polEq := c.AttachedPolicies.Equals(in.AttachedPolicies)
	nameEq := c.resourceName == in.resourceName

	// objIr may currently be nil in the case of k8s Services
	// TODO: add an IR for Services to avoid the need for this
	// see: internal/kgateway/extensions2/plugins/kubernetes/k8s.go
	objIrEq := true
	if c.ObjIr != nil {
		objIrEq = c.ObjIr.Equals(in.ObjIr)
	}

	return objEq && objVersionEq && objIrEq && polEq && nameEq
}

func (c BackendObjectIR) ClusterName() string {
	// TODO: fix this to somthing that's friendly to stats
	gvPrefix := c.GvPrefix
	if c.GvPrefix == "" {
		gvPrefix = strings.ToLower(c.Kind)
	}
	if c.ExtraKey != "" {
		return fmt.Sprintf("%s_%s_%s_%s_%d", gvPrefix, c.Namespace, c.Name, c.ExtraKey, c.Port)
	}
	return fmt.Sprintf("%s_%s_%s_%d", gvPrefix, c.Namespace, c.Name, c.Port)
	// return fmt.Sprintf("%s~%s:%d", c.GvPrefix, c.ObjectSource.ResourceName(), c.Port)
}

func (c BackendObjectIR) GetObjectSource() ObjectSource {
	return c.ObjectSource
}

func (c BackendObjectIR) GetObjectLabels() map[string]string {
	if c.Obj == nil {
		return make(map[string]string)
	}
	return c.Obj.GetLabels()
}

func (c BackendObjectIR) GetAttachedPolicies() AttachedPolicies {
	return c.AttachedPolicies
}

type Secret struct {
	// Ref to source object. sometimes the group and kind are not populated from api-server, so
	// set them explicitly here, and pass this around as the reference.
	// TODO: why does this have json tag?
	ObjectSource

	// original object. Opaque to us other than metadata.
	Obj metav1.Object

	Data map[string][]byte
}

func (c Secret) ResourceName() string {
	return c.ObjectSource.ResourceName()
}

func (c Secret) Equals(in Secret) bool {
	return c.ObjectSource.Equals(in.ObjectSource) && versionEquals(c.Obj, in.Obj)
}

var (
	_ krt.ResourceNamer   = Secret{}
	_ krt.Equaler[Secret] = Secret{}
	_ json.Marshaler      = Secret{}
)

func (l Secret) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name      string
		Namespace string
		Kind      string
		Data      string
	}{
		Name:      l.Name,
		Namespace: l.Namespace,
		Kind:      fmt.Sprintf("%T", l.Obj),
		Data:      "[REDACTED]",
	})
}

type Listener struct {
	gwv1.Listener
	Parent            client.Object
	AttachedPolicies  AttachedPolicies
	PolicyAncestorRef gwv1.ParentReference
}

func (listener Listener) GetParentReporter(reporter pluginsdkreporter.Reporter) pluginsdkreporter.GatewayReporter {
	switch t := listener.Parent.(type) {
	case *gwv1.Gateway:
		return reporter.Gateway(t)
	case *gwxv1.XListenerSet:
		return reporter.ListenerSet(t)
	}
	panic("Unknown parent type")
}

func (c Listener) Equals(in Listener) bool {
	return reflect.DeepEqual(c, in)
}

type Gateway struct {
	ObjectSource        `json:",inline"`
	Listeners           Listeners
	AllowedListenerSets ListenerSets
	DeniedListenerSets  ListenerSets
	Obj                 *gwv1.Gateway

	AttachedListenerPolicies AttachedPolicies
	AttachedHttpPolicies     AttachedPolicies

	PerConnectionBufferLimitBytes *uint32
}

func (c Gateway) ResourceName() string {
	return c.ObjectSource.ResourceName()
}

func (c Gateway) Equals(in Gateway) bool {
	return c.ObjectSource.Equals(in.ObjectSource) &&
		ptrEquals(c.PerConnectionBufferLimitBytes, in.PerConnectionBufferLimitBytes) &&
		versionEquals(c.Obj, in.Obj) &&
		c.AttachedListenerPolicies.Equals(in.AttachedListenerPolicies) &&
		c.AttachedHttpPolicies.Equals(in.AttachedHttpPolicies) &&
		c.Listeners.Equals(in.Listeners) &&
		c.AllowedListenerSets.Equals(in.AllowedListenerSets) &&
		c.DeniedListenerSets.Equals(in.DeniedListenerSets)
}

// Equals returns true if the two BackendRefIR instances are equal in cluster name, weight, backend object equality, and error.
func (a BackendRefIR) Equals(b BackendRefIR) bool {
	if a.ClusterName != b.ClusterName || a.Weight != b.Weight {
		return false
	}

	if !backendObjectEqual(a.BackendObject, b.BackendObject) {
		return false
	}

	if !errorsEqual(a.Err, b.Err) {
		return false
	}

	return true
}

func backendObjectEqual(a, b *BackendObjectIR) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equals(*b)
}

func errorsEqual(a, b error) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Error() == b.Error()
}

type ListenerSet struct {
	ObjectSource `json:",inline"`
	Listeners    Listeners
	Obj          *gwxv1.XListenerSet
	// ListenerSet polices are attached to the individual listeners in addition
	// to their specific policies
}

func (c ListenerSet) ResourceName() string {
	return c.ObjectSource.ResourceName()
}

func (c ListenerSet) Equals(in ListenerSet) bool {
	return c.ObjectSource.Equals(in.ObjectSource) && versionEquals(c.Obj, in.Obj) && c.Listeners.Equals(in.Listeners)
}

type ListenerSets []ListenerSet

func (c ListenerSets) Equals(in ListenerSets) bool {
	if len(c) != len(in) {
		return false
	}
	for i, ls := range c {
		if !ls.Equals(in[i]) {
			return false
		}
	}
	return true
}

type Listeners []Listener

func (c Listeners) Equals(in Listeners) bool {
	if len(c) != len(in) {
		return false
	}
	for i, l := range c {
		if !l.Equals(in[i]) {
			return false
		}
	}
	return true
}
