package deployer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// Gateways of a waypoint GatewayClass left over from before waypoint support was
// removed must not be deployed or translated as ingress gateways.
func TestRetiredWaypointClassGatewaysAreIgnored(t *testing.T) {
	ingressClass := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: wellknown.DefaultGatewayClassName},
		Spec:       gwv1.GatewayClassSpec{ControllerName: wellknown.DefaultGatewayControllerName},
	}
	waypointClass := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: wellknown.RetiredWaypointClassName},
		Spec:       gwv1.GatewayClassSpec{ControllerName: wellknown.DefaultGatewayControllerName},
	}
	renamedWaypointClass := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "custom-waypoint",
			Annotations: map[string]string{"ambient.istio.io/waypoint-inbound-binding": "PROXY/15088"},
		},
		Spec: gwv1.GatewayClassSpec{ControllerName: wellknown.DefaultGatewayControllerName},
	}
	gateway := func(name, class string) *gwv1.Gateway {
		return &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultNamespace},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: gwv1.ObjectName(class),
				Listeners:        []gwv1.Listener{{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType}},
			},
		}
	}

	commonCols := newCommonCols(t,
		ingressClass, waypointClass, renamedWaypointClass,
		gateway("ingress", ingressClass.Name),
		gateway("waypoint", waypointClass.Name),
		gateway("renamed-waypoint", renamedWaypointClass.Name),
	)

	var deployed []string
	for _, gw := range commonCols.GatewayIndex.GatewaysForDeployer.List() {
		deployed = append(deployed, gw.Name)
	}
	assert.Equal(t, []string{"ingress"}, deployed, "only the ingress Gateway should reach the deployer")

	var translated []string
	for _, gw := range commonCols.GatewayIndex.Gateways.List() {
		translated = append(translated, gw.Name)
	}
	assert.Equal(t, []string{"ingress"}, translated, "only the ingress Gateway should be translated")
}
