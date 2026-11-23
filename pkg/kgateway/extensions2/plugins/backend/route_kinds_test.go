package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// TestSupportedRouteKinds pins which Backend types are HTTP-only. A type that is
// realised through an HTTP filter must not be reachable from a TCPRoute or TLSRoute;
// a type that is a plain cluster must stay reachable from every route kind.
func TestSupportedRouteKinds(t *testing.T) {
	tcp := wellknown.TCPRouteGVK.GroupKind()
	http := wellknown.HTTPRouteGVK.GroupKind()

	tests := []struct {
		name     string
		spec     kgateway.BackendSpec
		httpOnly bool
	}{
		{
			name:     "static is a plain cluster",
			spec:     kgateway.BackendSpec{Static: &kgateway.StaticBackend{}},
			httpOnly: false,
		},
		{
			name:     "dynamic forward proxy needs its HTTP filter",
			spec:     kgateway.BackendSpec{DynamicForwardProxy: &kgateway.DynamicForwardProxyBackend{}},
			httpOnly: true,
		},
		{
			name:     "lambda signs requests in an HTTP filter",
			spec:     kgateway.BackendSpec{Aws: &kgateway.AwsBackend{Lambda: &kgateway.AwsLambda{}}},
			httpOnly: true,
		},
		{
			name:     "ec2 discovery is a plain cluster",
			spec:     kgateway.BackendSpec{Aws: &kgateway.AwsBackend{Ec2: &kgateway.AwsEc2{}}},
			httpOnly: false,
		},
		{
			name:     "gcp authenticates in an HTTP filter",
			spec:     kgateway.BackendSpec{Gcp: &kgateway.GcpBackend{}},
			httpOnly: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			be := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Backend", Namespace: "default", Name: "be"}, 0, "", ExtensionName)
			be.SupportedRouteKinds = supportedRouteKinds(&kgateway.Backend{Spec: tt.spec})

			assert.True(t, be.SupportsRouteKind(http), "every Backend type is reachable from an HTTPRoute")
			assert.Equal(t, !tt.httpOnly, be.SupportsRouteKind(tcp), "TCPRoute reachability")
			if tt.httpOnly {
				assert.Equal(t, []schema.GroupKind(ir.HTTPRouteKinds), be.SupportedRouteKinds)
			} else {
				assert.Nil(t, be.SupportedRouteKinds, "a plain cluster declares no restriction")
			}
		})
	}
}

// TestInlineEndpoints pins which Backend types carry their endpoints in their
// own cluster, so a UDPRoute rule cannot weight them with other backends.
func TestInlineEndpoints(t *testing.T) {
	tests := []struct {
		name   string
		spec   kgateway.BackendSpec
		inline bool
	}{
		{
			name:   "static inlines its hosts",
			spec:   kgateway.BackendSpec{Static: &kgateway.StaticBackend{}},
			inline: true,
		},
		{
			name:   "priority groups inline their members' hosts",
			spec:   kgateway.BackendSpec{PriorityGroups: []kgateway.PriorityGroup{{}}},
			inline: true,
		},
		{
			name:   "ec2 discovery has endpoint rows",
			spec:   kgateway.BackendSpec{Aws: &kgateway.AwsBackend{Ec2: &kgateway.AwsEc2{}}},
			inline: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.inline, inlineEndpoints(&kgateway.Backend{Spec: tt.spec}))
		})
	}
}
