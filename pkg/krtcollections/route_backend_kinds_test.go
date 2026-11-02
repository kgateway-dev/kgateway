package krtcollections

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// TestRouteBackendSupportedRouteKinds pins that a backend's SupportedRouteKinds declaration
// is enforced on every route kind that resolves backendRefs, not only the ones the in-tree
// Backend plugin happens to exercise. The backend under test declares HTTPRoute only, which
// is narrower than ir.HTTPRouteKinds, so GRPCRoute must be rejected too.
func TestRouteBackendSupportedRouteKinds(t *testing.T) {
	httpOnly := httpOnlyBackend()

	tests := []struct {
		name     string
		route    any
		routeGK  schema.GroupKind
		routeNN  string
		rejected bool
	}{
		{
			name:    "HTTPRoute resolves a backend that supports HTTPRoute",
			route:   httpRouteWithBackendRef(httpOnly.Name, "", nil),
			routeGK: wellknown.HTTPRouteGVK.GroupKind(),
			routeNN: "httproute",
		},
		{
			name:     "GRPCRoute is rejected by a backend that supports HTTPRoute only",
			route:    grpcRouteWithBackendRef(httpOnly.Name),
			routeGK:  wellknown.GRPCRouteGVK.GroupKind(),
			routeNN:  "grpcroute",
			rejected: true,
		},
		{
			name:     "TCPRoute is rejected by a backend that supports HTTPRoute only",
			route:    tcpRouteWithKgatewayBackendRef(httpOnly.Name),
			routeGK:  wellknown.TCPRouteGVK.GroupKind(),
			routeNN:  "tcproute",
			rejected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rtidx := preRouteIndex(t, []any{httpOnly, tt.route})
			wrapper := rtidx.Fetch(krt.TestingDummyContext{}, tt.routeGK, "default", tt.routeNN)
			require.NotNil(t, wrapper, "route should be in the index")
			backends := getBackends(wrapper.Route)
			require.Len(t, backends, 1)
			b := backends[0]

			if !tt.rejected {
				require.NoError(t, b.Err)
				require.NotNil(t, b.BackendObject, "a supported route kind resolves the backend")
				assert.Equal(t, b.BackendObject.ClusterName(), b.ClusterName)
				return
			}

			var unsupported *UnsupportedRouteKindError
			require.ErrorAs(t, b.Err, &unsupported, "the reference must fail with UnsupportedRouteKindError, got %v", b.Err)
			assert.Equal(t, tt.routeGK, unsupported.RouteKind)
			assert.Equal(t, []schema.GroupKind{wellknown.HTTPRouteGVK.GroupKind()}, unsupported.Supported)
			assert.Nil(t, b.BackendObject, "a rejected reference must not carry the backend")
			assert.Equal(t, wellknown.BlackholeClusterName, b.ClusterName, "a rejected reference routes to the blackhole cluster")
			assert.False(t, errors.Is(b.Err, ErrUnknownBackendKind), "the backend kind itself is known")
		})
	}
}

// supportedRouteKindsLabel lets a test Backend declare its SupportedRouteKinds. The value is
// a comma-separated list of route kinds in the Gateway API group; see backendUpstreams.
const supportedRouteKindsLabel = "test.kgateway.dev/supported-route-kinds"

func httpOnlyBackend() *kgateway.Backend {
	be := backend("")
	be.Name = "http-only-backend"
	be.Labels = map[string]string{supportedRouteKindsLabel: wellknown.HTTPRouteKind}
	return be
}

func grpcRouteWithBackendRef(refN string) *gwv1.GRPCRoute {
	return &gwv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grpcroute",
			Namespace: "default",
		},
		Spec: gwv1.GRPCRouteSpec{
			Rules: []gwv1.GRPCRouteRule{
				{
					BackendRefs: []gwv1.GRPCBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: kgatewayBackendRef(refN),
							},
						},
					},
				},
			},
		},
	}
}

func tcpRouteWithKgatewayBackendRef(refN string) *gwv1a2.TCPRoute {
	return &gwv1a2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcproute",
			Namespace: "default",
		},
		Spec: gwv1a2.TCPRouteSpec{
			Rules: []gwv1a2.TCPRouteRule{
				{
					BackendRefs: []gwv1.BackendRef{
						{
							BackendObjectReference: kgatewayBackendRef(refN),
						},
					},
				},
			},
		},
	}
}

func kgatewayBackendRef(refN string) gwv1.BackendObjectReference {
	return gwv1.BackendObjectReference{
		Group: new(gwv1.Group(wellknown.BackendGVK.Group)),
		Kind:  new(gwv1.Kind(wellknown.BackendGVK.Kind)),
		Name:  gwv1.ObjectName(refN),
	}
}
