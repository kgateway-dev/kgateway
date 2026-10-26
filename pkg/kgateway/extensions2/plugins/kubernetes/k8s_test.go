package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/ptr"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestBuildServiceBackendObjectIRAppProtocol(t *testing.T) {
	tests := []struct {
		name string
		port corev1.ServicePort
		want ir.AppProtocol
	}{
		{name: "explicit http", port: corev1.ServicePort{Name: "web", AppProtocol: new("http")}, want: ir.HTTP1AppProtocol},
		{name: "explicit HTTP", port: corev1.ServicePort{Name: "web", AppProtocol: new("HTTP")}, want: ir.HTTP1AppProtocol},
		{name: "port named http is the default", port: corev1.ServicePort{Name: "http"}, want: ir.DefaultAppProtocol},
		{name: "explicit appProtocol wins over the port name", port: corev1.ServicePort{Name: "http", AppProtocol: new("kubernetes.io/h2c")}, want: ir.HTTP2AppProtocol},
		{name: "port name is the fallback", port: corev1.ServicePort{Name: "grpc"}, want: ir.HTTP2AppProtocol},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.port.Port = 8080
			svc := &corev1.Service{Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{tt.port}}}
			be := BuildServiceBackendObjectIR(svc, tt.port.Port, ptr.OrDefault(tt.port.AppProtocol, tt.port.Name))
			assert.Equal(t, tt.want, be.AppProtocol)
		})
	}
}
