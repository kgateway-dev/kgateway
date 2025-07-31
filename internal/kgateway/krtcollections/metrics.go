package krtcollections

import (
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	resourcesSubsystem = "resources"
)

var (
	resourcesManaged = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: resourcesSubsystem,
			Name:      "managed",
			Help:      "Current number of resources managed",
		},
		[]string{"namespace", "parent", "resource"},
	)
)

type resourceMetricLabels struct {
	Namespace string
	Parent    string
	Resource  string
}

func (r resourceMetricLabels) toMetricsLabels() []metrics.Label {
	return []metrics.Label{
		{Name: "namespace", Value: r.Namespace},
		{Name: "parent", Value: r.Parent},
		{Name: "resource", Value: r.Resource},
	}
}

// GetResourceMetricEventHandler returns a function that handles krt events for various Gateway API resources.
func GetResourceMetricEventHandler[T any]() func(krt.Event[T]) {
	var (
		eventType       controllers.EventType
		resourceType    string
		names           []string
		namesOld        []string
		namespace       string
		namespaceOld    string
		clientObject    any
		clientObjectOld any
	)

	return func(o krt.Event[T]) {
		clientObject = o.Latest()
		eventType = o.Event

		// If the event is an update, we must decrement resource metrics using the old label
		// values before incrementing the resource count with the new label values.
		if eventType == controllers.EventUpdate && o.Old != nil {
			clientObjectOld = *o.Old
		}

		switch obj := clientObject.(type) {
		case ir.PolicyWrapper:
			resourceType = obj.Kind
			namespace = obj.Policy.GetNamespace()
			names = make([]string, 0, len(obj.TargetRefs))

			for _, ref := range obj.TargetRefs {
				if ref.Group == wellknown.GatewayGroup && ref.Kind == wellknown.GatewayKind {
					names = append(names, ref.Name)
				}
			}

			if len(names) == 0 {
				names = []string{""}
			}

			if clientObjectOld != nil {
				namespaceOld = clientObjectOld.(ir.PolicyWrapper).Namespace
				namesOld = make([]string, 0, len(clientObjectOld.(ir.PolicyWrapper).TargetRefs))

				for _, ref := range clientObjectOld.(ir.PolicyWrapper).TargetRefs {
					if ref.Group == wellknown.GatewayGroup && ref.Kind == wellknown.GatewayKind {
						namesOld = append(namesOld, ref.Name)
					}

					if len(namesOld) == 0 {
						namesOld = []string{""}
					}
				}
			}
		case *gwv1.HTTPRoute:
			resourceType = "HTTPRoute"
			namespace = obj.Namespace
			names = make([]string, 0, len(obj.Spec.ParentRefs))
			for _, pr := range obj.Spec.ParentRefs {
				names = append(names, string(pr.Name))
			}

			if clientObjectOld != nil {
				oldObj := clientObjectOld.(*gwv1.HTTPRoute)
				namespaceOld = oldObj.Namespace
				namesOld = make([]string, 0, len(oldObj.Spec.ParentRefs))
				for _, pr := range oldObj.Spec.ParentRefs {
					namesOld = append(namesOld, string(pr.Name))
				}
			}
		case *gwv1a2.TCPRoute:
			resourceType = "TCPRoute"
			namespace = obj.Namespace
			names = make([]string, 0, len(obj.Spec.ParentRefs))
			for _, pr := range obj.Spec.ParentRefs {
				names = append(names, string(pr.Name))
			}

			if clientObjectOld != nil {
				oldObj := clientObjectOld.(*gwv1a2.TCPRoute)
				namespaceOld = oldObj.Namespace
				namesOld = make([]string, 0, len(oldObj.Spec.ParentRefs))
				for _, pr := range oldObj.Spec.ParentRefs {
					namesOld = append(namesOld, string(pr.Name))
				}
			}
		case *gwv1a2.TLSRoute:
			resourceType = "TLSRoute"
			namespace = obj.Namespace
			names = make([]string, 0, len(obj.Spec.ParentRefs))
			for _, pr := range obj.Spec.ParentRefs {
				names = append(names, string(pr.Name))
			}

			if clientObjectOld != nil {
				oldObj := clientObjectOld.(*gwv1a2.TLSRoute)
				namespaceOld = oldObj.Namespace
				namesOld = make([]string, 0, len(oldObj.Spec.ParentRefs))
				for _, pr := range oldObj.Spec.ParentRefs {
					namesOld = append(namesOld, string(pr.Name))
				}
			}
		case *gwv1.GRPCRoute:
			resourceType = "GRPCRoute"
			namespace = obj.Namespace
			names = make([]string, 0, len(obj.Spec.ParentRefs))
			for _, pr := range obj.Spec.ParentRefs {
				names = append(names, string(pr.Name))
			}

			if clientObjectOld != nil {
				oldObj := clientObjectOld.(*gwv1.GRPCRoute)
				namespaceOld = oldObj.Namespace
				namesOld = make([]string, 0, len(oldObj.Spec.ParentRefs))
				for _, pr := range oldObj.Spec.ParentRefs {
					namesOld = append(namesOld, string(pr.Name))
				}
			}
		case *gwv1.Gateway:
			resourceType = "Gateway"
			namespace = obj.Namespace
			names = []string{obj.Name}

			if clientObjectOld != nil {
				namespaceOld = clientObjectOld.(*gwv1.Gateway).Namespace
				namesOld = []string{clientObjectOld.(*gwv1.Gateway).Name}
			}
		case *gwxv1a1.XListenerSet:
			resourceType = "XListenerSet"
			namespace = obj.Namespace
			names = []string{string(obj.Spec.ParentRef.Name)}

			if clientObjectOld != nil {
				namespaceOld = clientObjectOld.(*gwxv1a1.XListenerSet).Namespace
				namesOld = []string{string(clientObjectOld.(*gwxv1a1.XListenerSet).Spec.ParentRef.Name)}
			}
		}

		switch eventType {
		case controllers.EventAdd:
			for _, name := range names {
				resourcesManaged.Add(1, resourceMetricLabels{
					Parent:    name,
					Namespace: namespace,
					Resource:  resourceType,
				}.toMetricsLabels()...)
			}
		case controllers.EventUpdate:
			for _, name := range namesOld {
				resourcesManaged.Sub(1, resourceMetricLabels{
					Parent:    name,
					Namespace: namespaceOld,
					Resource:  resourceType,
				}.toMetricsLabels()...)
			}

			for _, name := range names {
				resourcesManaged.Add(1, resourceMetricLabels{
					Parent:    name,
					Namespace: namespace,
					Resource:  resourceType,
				}.toMetricsLabels()...)
			}
		case controllers.EventDelete:
			for _, name := range names {
				resourcesManaged.Sub(1, resourceMetricLabels{
					Parent:    name,
					Namespace: namespace,
					Resource:  resourceType,
				}.toMetricsLabels()...)
			}
		}
	}
}

// ResetMetrics resets the metrics from this package.
// This is provided for testing purposes only.
func ResetMetrics() {
	resourcesManaged.Reset()
}
