package fake

import (
	"testing"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/config/schema/gvr"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func TestKindForSeededCRD(t *testing.T) {
	tests := []struct {
		name string
		gvr  schema.GroupVersionResource
		want string
	}{
		{name: "backend tls policy", gvr: gvr.BackendTLSPolicy, want: wellknown.BackendTLSPolicyKind},
		{name: "grpc route", gvr: gvr.GRPCRoute, want: wellknown.GRPCRouteKind},
		{name: "tls route v1alpha3", gvr: wellknown.TLSRouteV1Alpha3GVR, want: wellknown.TLSRouteKind},
		{name: "listener policy", gvr: wellknown.ListenerPolicyGVR, want: wellknown.ListenerPolicyGVK.Kind},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, kindForSeededCRD(tt.gvr))
		})
	}
}
