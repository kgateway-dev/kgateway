package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestServicePortProtocol(t *testing.T) {
	tests := []struct {
		name string
		port corev1.ServicePort
		want string
	}{
		{name: "explicit http", port: corev1.ServicePort{Name: "web", AppProtocol: new("http")}, want: "http"},
		{name: "port named http is not a choice of HTTP/1.1", port: corev1.ServicePort{Name: "http"}, want: ""},
		{name: "port named HTTP is not a choice of HTTP/1.1", port: corev1.ServicePort{Name: "HTTP"}, want: ""},
		{name: "explicit appProtocol wins over the port name", port: corev1.ServicePort{Name: "http", AppProtocol: new("kubernetes.io/h2c")}, want: "kubernetes.io/h2c"},
		{name: "port name is the fallback", port: corev1.ServicePort{Name: "grpc"}, want: "grpc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, servicePortProtocol(tt.port))
		})
	}
}
