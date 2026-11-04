package wellknown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestIsRetiredWaypointClass(t *testing.T) {
	tests := []struct {
		name string
		gwc  *gwv1.GatewayClass
		want bool
	}{
		{
			name: "default waypoint class name",
			gwc:  &gwv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: RetiredWaypointClassName}},
			want: true,
		},
		{
			name: "renamed waypoint class with the inbound binding annotation",
			gwc: &gwv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{
				Name:        "custom-waypoint",
				Annotations: map[string]string{waypointInboundBindingAnnotation: "PROXY/15088"},
			}},
			want: true,
		},
		{
			name: "ingress class",
			gwc:  &gwv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: DefaultGatewayClassName}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsRetiredWaypointClass(tt.gwc))
		})
	}
}
