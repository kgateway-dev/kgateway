package krtcollections

import (
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

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
		gatewayNames    []string
		gatewayNamesOld []string
		namespace       string
		namespaceOld    string
		clientObject    any
		clientObjectOld any
	)

	return func(o krt.Event[T]) {
		clientObject = o.Latest()
		namespace = clientObject.(client.Object).GetNamespace()
		eventType = o.Event

		// If the event is an update, we must decrement resource metrics using the old label
		// values before incrementing the resource count with the new label values.
		if eventType == controllers.EventUpdate && o.Old != nil {
			clientObjectOld = *o.Old
			namespaceOld = clientObjectOld.(client.Object).GetNamespace()
		}

		switch obj := clientObject.(type) {
		case *gwv1.HTTPRoute:
			resourceType = "HTTPRoute"
			gatewayNames = make([]string, 0, len(obj.Spec.ParentRefs))
			for _, pr := range obj.Spec.ParentRefs {
				gatewayNames = append(gatewayNames, string(pr.Name))
			}

			if clientObjectOld != nil {
				oldObj := clientObjectOld.(*gwv1.HTTPRoute)
				gatewayNamesOld = make([]string, 0, len(oldObj.Spec.ParentRefs))
				for _, pr := range oldObj.Spec.ParentRefs {
					gatewayNamesOld = append(gatewayNamesOld, string(pr.Name))
				}
			}
		case *gwv1a2.TCPRoute:
			resourceType = "TCPRoute"
			gatewayNames = make([]string, 0, len(obj.Spec.ParentRefs))
			for _, pr := range obj.Spec.ParentRefs {
				gatewayNames = append(gatewayNames, string(pr.Name))
			}

			if clientObjectOld != nil {
				oldObj := clientObjectOld.(*gwv1a2.TCPRoute)
				gatewayNamesOld = make([]string, 0, len(oldObj.Spec.ParentRefs))
				for _, pr := range oldObj.Spec.ParentRefs {
					gatewayNamesOld = append(gatewayNamesOld, string(pr.Name))
				}
			}
		case *gwv1a2.TLSRoute:
			resourceType = "TLSRoute"
			gatewayNames = make([]string, 0, len(obj.Spec.ParentRefs))
			for _, pr := range obj.Spec.ParentRefs {
				gatewayNames = append(gatewayNames, string(pr.Name))
			}

			if clientObjectOld != nil {
				oldObj := clientObjectOld.(*gwv1a2.TLSRoute)
				gatewayNamesOld = make([]string, 0, len(oldObj.Spec.ParentRefs))
				for _, pr := range oldObj.Spec.ParentRefs {
					gatewayNamesOld = append(gatewayNamesOld, string(pr.Name))
				}
			}
		case *gwv1.GRPCRoute:
			resourceType = "GRPCRoute"
			gatewayNames = make([]string, 0, len(obj.Spec.ParentRefs))
			for _, pr := range obj.Spec.ParentRefs {
				gatewayNames = append(gatewayNames, string(pr.Name))
			}

			if clientObjectOld != nil {
				oldObj := clientObjectOld.(*gwv1.GRPCRoute)
				gatewayNamesOld = make([]string, 0, len(oldObj.Spec.ParentRefs))
				for _, pr := range oldObj.Spec.ParentRefs {
					gatewayNamesOld = append(gatewayNamesOld, string(pr.Name))
				}
			}
		case *gwv1.Gateway:
			resourceType = "Gateway"
			gatewayNames = []string{clientObject.(client.Object).GetName()}

			if clientObjectOld != nil {
				gatewayNamesOld = []string{clientObjectOld.(*gwv1.Gateway).Name}
			}
		case *gwxv1a1.XListenerSet:
			resourceType = "XListenerSet"
			namespace = clientObject.(client.Object).GetNamespace()
			gatewayNames = []string{string(obj.Spec.ParentRef.Name)}

			if clientObjectOld != nil {
				gatewayNamesOld = []string{string(clientObjectOld.(*gwxv1a1.XListenerSet).Spec.ParentRef.Name)}
			}
		default:
			return
		}

		switch eventType {
		case controllers.EventAdd:
			for _, gatewayName := range gatewayNames {
				resourcesManaged.Add(1, resourceMetricLabels{
					Parent:    gatewayName,
					Namespace: namespace,
					Resource:  resourceType,
				}.toMetricsLabels()...)
			}
		case controllers.EventUpdate:
			for _, gatewayName := range gatewayNamesOld {
				resourcesManaged.Sub(1, resourceMetricLabels{
					Parent:    gatewayName,
					Namespace: namespaceOld,
					Resource:  resourceType,
				}.toMetricsLabels()...)
			}

			for _, gatewayName := range gatewayNames {
				resourcesManaged.Add(1, resourceMetricLabels{
					Parent:    gatewayName,
					Namespace: namespace,
					Resource:  resourceType,
				}.toMetricsLabels()...)
			}
		case controllers.EventDelete:
			for _, gatewayName := range gatewayNames {
				resourcesManaged.Sub(1, resourceMetricLabels{
					Parent:    gatewayName,
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
