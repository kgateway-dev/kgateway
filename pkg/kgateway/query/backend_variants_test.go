package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestBuildGatewayBackendClientCertificateVariantsAndRewriteRoutes(t *testing.T) {
	gateway := &ir.Gateway{
		ObjectSource: ir.ObjectSource{
			Group:     gwv1.GroupVersion.Group,
			Kind:      "Gateway",
			Namespace: "default",
			Name:      "gw",
		},
	}
	clientCertificate := &ir.GatewayBackendClientCertificateIR{
		Certificate: ir.TLSCertificate{
			CertChain:  []byte("cert"),
			PrivateKey: []byte("key"),
		},
	}

	parentBackend := testBackendObjectIR("parent-svc", 80)
	childBackend := testBackendObjectIR("child-svc", 8080)
	delegateRef := ir.ObjectSource{
		Group:     gwv1.GroupVersion.Group,
		Kind:      "HTTPRoute",
		Namespace: "default",
		Name:      "child-route",
	}

	childRoute := &RouteInfo{
		Object: &ir.HttpRouteIR{
			ObjectSource: ir.ObjectSource{
				Group:     gwv1.GroupVersion.Group,
				Kind:      "HTTPRoute",
				Namespace: "default",
				Name:      "child-route",
			},
			Rules: []ir.HttpRouteRuleIR{
				{
					Backends: []ir.HttpBackendOrDelegate{
						{
							Backend: &ir.BackendRefIR{
								BackendObject: &childBackend,
								ClusterName:   childBackend.ClusterName(),
								Weight:        1,
							},
						},
					},
				},
			},
		},
	}

	parentRoute := &RouteInfo{
		Object: &ir.HttpRouteIR{
			ObjectSource: ir.ObjectSource{
				Group:     gwv1.GroupVersion.Group,
				Kind:      "HTTPRoute",
				Namespace: "default",
				Name:      "parent-route",
			},
			Rules: []ir.HttpRouteRuleIR{
				{
					Backends: []ir.HttpBackendOrDelegate{
						{
							Backend: &ir.BackendRefIR{
								BackendObject: &parentBackend,
								ClusterName:   parentBackend.ClusterName(),
								Weight:        1,
							},
						},
						{
							Delegate: &delegateRef,
						},
					},
				},
			},
		},
		Children: NewBackendMap[[]*RouteInfo](),
	}
	parentRoute.Children.Add(delegateRef, []*RouteInfo{childRoute})

	routes := NewRoutesForGwResult()
	routes.setListenerResult(nil, "listener", &ListenerResult{
		Routes: []*RouteInfo{parentRoute},
	})

	variants := BuildGatewayBackendClientCertificateVariants(routes, gateway, clientCertificate)
	require.Len(t, variants, 2)

	parentVariant := variants[parentBackend.ResourceName()]
	require.NotNil(t, parentVariant)
	assert.NotEqual(t, parentBackend.ClusterName(), parentVariant.ClusterName())

	childVariant := variants[childBackend.ResourceName()]
	require.NotNil(t, childVariant)
	assert.NotEqual(t, childBackend.ClusterName(), childVariant.ClusterName())

	rewritten := RewriteRoutesForBackendVariants(routes, variants)
	rewrittenListener := rewritten.GetListenerResult(nil, "listener")
	require.NotNil(t, rewrittenListener)
	require.Len(t, rewrittenListener.Routes, 1)

	rewrittenParent := rewrittenListener.Routes[0].Object.(*ir.HttpRouteIR)
	assert.Equal(t, parentVariant.ClusterName(), rewrittenParent.Rules[0].Backends[0].Backend.ClusterName)
	assert.Equal(t, parentBackend.ClusterName(), parentRoute.Object.(*ir.HttpRouteIR).Rules[0].Backends[0].Backend.ClusterName)

	children, err := rewrittenListener.Routes[0].GetChildrenForRef(delegateRef)
	require.NoError(t, err)
	require.Len(t, children, 1)
	rewrittenChild := children[0].Object.(*ir.HttpRouteIR)
	assert.Equal(t, childVariant.ClusterName(), rewrittenChild.Rules[0].Backends[0].Backend.ClusterName)
	assert.Equal(t, childBackend.ClusterName(), childRoute.Object.(*ir.HttpRouteIR).Rules[0].Backends[0].Backend.ClusterName)
}

func testBackendObjectIR(name string, port int32) ir.BackendObjectIR {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{
		Kind:      "Service",
		Namespace: "default",
		Name:      name,
	}, port, "")
	backend.Obj = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       "default",
			ResourceVersion: "1",
			Generation:      1,
		},
	}
	backend.GvPrefix = "kube"
	return backend
}
