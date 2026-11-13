package krtcollections_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// A ReferenceGrant added after the route is translated must reach the route IR: the route's
// generation does not change, so the mirror's reference error has to take part in equality.
func TestHTTPRouteMirrorReferenceGrantAddedLater(t *testing.T) {
	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "mirror-route", Namespace: "default", Generation: 1},
		Spec: gwv1.HTTPRouteSpec{
			Rules: []gwv1.HTTPRouteRule{{
				Filters: []gwv1.HTTPRouteFilter{{
					Type: gwv1.HTTPRouteFilterRequestMirror,
					RequestMirror: &gwv1.HTTPRequestMirrorFilter{
						BackendRef: gwv1.BackendObjectReference{
							Name:      "mirror-svc",
							Namespace: new(gwv1.Namespace("other")),
							Port:      new(gwv1.PortNumber(8080)),
						},
					},
				}},
			}},
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "mirror-svc", Namespace: "other"},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 8080}}},
	}
	grant := &gwv1b1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-mirror", Namespace: "other"},
		Spec: gwv1b1.ReferenceGrantSpec{
			From: []gwv1b1.ReferenceGrantFrom{{Group: gwv1.GroupName, Kind: "HTTPRoute", Namespace: "default"}},
			To:   []gwv1b1.ReferenceGrantTo{{Group: "", Kind: "Service"}},
		},
	}

	mock := krttest.NewMock(t, []any{route, svc})
	services := krttest.GetMockCollection[*corev1.Service](mock)
	grants := krt.NewMutableCollection[*gwv1b1.ReferenceGrant](nil, nil)
	refgrants := krtcollections.NewRefGrantIndex(grants.AsCollection(), apisettings.ReferenceGrantPermissive)
	policies := krtcollections.NewPolicyIndex(krtutil.KrtOptions{}, sdk.ContributesPolicies{}, apisettings.Settings{})
	backends := krtcollections.NewBackendIndex(krtutil.KrtOptions{}, policies, refgrants)
	backends.AddBackends(schema.GroupKind{Group: "", Kind: "Service"}, k8sSvcUpstreams(services))
	routesIndex := krtcollections.NewRoutesIndex(
		krtutil.KrtOptions{},
		krttest.GetMockCollection[*gwv1.HTTPRoute](mock),
		krttest.GetMockCollection[*gwv1.GRPCRoute](mock),
		krttest.GetMockCollection[*gwv1a2.TCPRoute](mock),
		krttest.GetMockCollection[*gwv1a2.TLSRoute](mock),
		policies,
		backends,
		refgrants,
		apisettings.Settings{},
	)
	services.WaitUntilSynced(context.Background().Done())
	for !routesIndex.HasSynced() {
		time.Sleep(time.Second / 10)
	}

	rule := func() ir.HttpRouteRuleIR {
		rw := routesIndex.Fetch(krt.TestingDummyContext{},
			schema.GroupKind{Group: gwv1.GroupName, Kind: "HTTPRoute"}, route.Namespace, route.Name)
		require.NotNil(t, rw, "expected the HTTPRoute to be indexed")
		httpRouteIR, ok := rw.Route.(*ir.HttpRouteIR)
		require.True(t, ok, "expected route to be HttpRouteIR")
		require.Len(t, httpRouteIR.Rules, 1)
		return httpRouteIR.Rules[0]
	}
	mirrorCluster := func(r ir.HttpRouteRuleIR) string {
		atts := r.ExtensionRefs.Policies[ir.VirtualBuiltInGK]
		require.Len(t, atts, 1, "expected the mirror filter to be attached")
		return krtcollections.MirrorClusterForTest(atts[0].PolicyIr)
	}

	denied := rule()
	assert.Empty(t, mirrorCluster(denied), "mirror should be dropped without a ReferenceGrant")
	assert.ErrorIs(t, denied.ResolvedRefsErr, krtcollections.ErrMissingReferenceGrant)
	assert.NoError(t, denied.Err, "a denied mirror must not replace the route")

	grants.Reset([]*gwv1b1.ReferenceGrant{grant})
	assert.Eventually(t, func() bool {
		r := rule()
		return r.ResolvedRefsErr == nil && mirrorCluster(r) == "service_other_mirror-svc_8080"
	}, 5*time.Second, 50*time.Millisecond, "the ReferenceGrant should enable the mirror and clear the error")
}
