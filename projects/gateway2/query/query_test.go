package query_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/solo-io/gloo/pkg/schemes"
	"github.com/solo-io/gloo/projects/gateway2/query"
)

// Helper for test setup
func setup() (*runtime.Scheme, *fake.ClientBuilder) {
	scheme := schemes.DefaultScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)

	query.IterateIndices(func(o client.Object, field string, indexerFunc client.IndexerFunc) error {
		builder.WithIndex(o, field, indexerFunc)
		return nil
	})

	return scheme, builder
}

// Test Group: Backend Reference Tests
func TestBackendReference(t *testing.T) {
	ctx := context.Background()

	t.Run("Same Namespace", func(t *testing.T) {
		scheme, builder := setup()
		fakeClient := builder.WithObjects(svc("default")).Build()
		gq := query.NewData(fakeClient, scheme)

		ref := &gwv1.BackendObjectReference{
			Name: "foo",
		}

		backend, err := gq.GetBackendForRef(ctx, toFrom(scheme, httpRoute()), ref)
		assert.NoError(t, err)
		assert.NotNil(t, backend)
		assert.Equal(t, "foo", backend.GetName())
		assert.Equal(t, "default", backend.GetNamespace())
	})

	t.Run("Different Namespace with ReferenceGrant", func(t *testing.T) {
		scheme, builder := setup()

		rg := refGrant()
		fakeClient := builder.WithObjects(svc("default2"), rg).Build()
		gq := query.NewData(fakeClient, scheme)

		ref := &gwv1.BackendObjectReference{
			Name:      "foo",
			Namespace: ptr.To(gwv1.Namespace("default2")),
		}

		backend, err := gq.GetBackendForRef(ctx, toFrom(scheme, httpRoute()), ref)
		assert.NoError(t, err)
		assert.NotNil(t, backend)
		assert.Equal(t, "foo", backend.GetName())
		assert.Equal(t, "default2", backend.GetNamespace())
	})

	t.Run("Not Found with Valid ReferenceGrant", func(t *testing.T) {
		scheme, builder := setup()

		rg := refGrant()
		fakeClient := builder.WithObjects(rg).Build()
		gq := query.NewData(fakeClient, scheme)

		ref := &gwv1.BackendObjectReference{
			Name:      "foo",
			Namespace: ptr.To(gwv1.Namespace("default2")),
		}

		backend, err := gq.GetBackendForRef(ctx, toFrom(scheme, httpRoute()), ref)
		assert.Error(t, err)
		assert.True(t, apierrors.IsNotFound(err))
		assert.Nil(t, backend)
	})

	t.Run("Fail to Get Service with Invalid ReferenceGrant From Configuration", func(t *testing.T) {
		scheme, builder := setup()

		rg := &gwv1b1.ReferenceGrant{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default2",
				Name:      "foo",
			},
			Spec: gwv1b1.ReferenceGrantSpec{
				From: []gwv1b1.ReferenceGrantFrom{
					{
						Group:     gwv1.Group("gateway.networking.k8s.io"),
						Kind:      gwv1.Kind("NotGateway"),
						Namespace: gwv1.Namespace("default"),
					},
					{
						Group:     gwv1.Group("gateway.networking.k8s.io"),
						Kind:      gwv1.Kind("Gateway"),
						Namespace: gwv1.Namespace("default2"),
					},
				},
				To: []gwv1b1.ReferenceGrantTo{
					{
						Group: gwv1.Group("core"),
						Kind:  gwv1.Kind("Service"),
					},
				},
			},
		}

		fakeClient := builder.WithObjects(rg, svc("default2")).Build()
		gq := query.NewData(fakeClient, scheme)

		ref := &gwv1.BackendObjectReference{
			Name:      "foo",
			Namespace: ptr.To(gwv1.Namespace("default2")),
		}

		backend, err := gq.GetBackendForRef(ctx, toFrom(scheme, httpRoute()), ref)
		assert.ErrorIs(t, err, query.ErrMissingReferenceGrant)
		assert.Nil(t, backend)
	})

	t.Run("Invalid ReferenceGrant Configuration", func(t *testing.T) {
		scheme, builder := setup()

		rg := refGrant()
		rg.Spec.From[0].Kind = "NotGateway"
		fakeClient := builder.WithObjects(rg, svc("default2")).Build()
		gq := query.NewData(fakeClient, scheme)

		ref := &gwv1.BackendObjectReference{
			Name:      "foo",
			Namespace: ptr.To(gwv1.Namespace("default2")),
		}

		backend, err := gq.GetBackendForRef(ctx, toFrom(scheme, httpRoute()), ref)
		assert.ErrorIs(t, err, query.ErrMissingReferenceGrant)
		assert.Nil(t, backend)
	})

	t.Run("Fail to Get Service with Missing ReferenceGrant", func(t *testing.T) {
		scheme, builder := setup()
		fakeClient := builder.WithObjects(svc("default3")).Build()
		gq := query.NewData(fakeClient, scheme)

		ref := &gwv1.BackendObjectReference{
			Name:      "foo",
			Namespace: ptr.To(gwv1.Namespace("default3")),
		}

		backend, err := gq.GetBackendForRef(ctx, toFrom(scheme, httpRoute()), ref)
		assert.ErrorIs(t, err, query.ErrMissingReferenceGrant)
		assert.Nil(t, backend)
	})

	t.Run("Fail to Get Service with ReferenceGrant in Wrong Namespace", func(t *testing.T) {
		scheme, builder := setup()

		rg := refGrant()
		fakeClient := builder.WithObjects(svc("default3"), rg).Build()
		gq := query.NewData(fakeClient, scheme)

		ref := &gwv1.BackendObjectReference{
			Name:      "foo",
			Namespace: ptr.To(gwv1.Namespace("default3")),
		}

		backend, err := gq.GetBackendForRef(ctx, toFrom(scheme, httpRoute()), ref)
		assert.ErrorIs(t, err, query.ErrMissingReferenceGrant)
		assert.Nil(t, backend)
	})
}

// Test Group: Secret Reference Tests
func TestGetSecretRef(t *testing.T) {
	ctx := context.Background()

	t.Run("Get Secret from Different Namespace with ReferenceGrant", func(t *testing.T) {
		scheme, builder := setup()

		rg := refGrantSecret()
		fakeClient := builder.WithObjects(secret("default2"), rg).Build()
		gq := query.NewData(fakeClient, scheme)

		ref := gwv1.SecretObjectReference{
			Name:      "foo",
			Namespace: ptr.To(gwv1.Namespace("default2")),
		}

		secret, err := gq.GetSecretForRef(ctx, toFrom(scheme, gw()), ref)
		assert.NoError(t, err)
		assert.NotNil(t, secret)
		assert.Equal(t, "foo", secret.GetName())
		assert.Equal(t, "default2", secret.GetNamespace())
	})
}

// Test Group: Routes and Listeners
func TestRoutesForGateway(t *testing.T) {
	ctx := context.Background()

	t.Run("HTTPRoutes for Listener", func(t *testing.T) {
		scheme, builder := setup()

		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo",
				Protocol: gwv1.HTTPProtocolType,
			},
		}

		hr := httpRoute()
		hr.Spec.ParentRefs = []gwv1.ParentReference{
			{
				Name: "test",
			},
		}

		fakeClient := builder.WithObjects(hr).Build()
		gq := query.NewData(fakeClient, scheme)

		routes, err := gq.GetRoutesForGateway(ctx, gw)
		assert.NoError(t, err)
		assert.NoError(t, routes.ListenerResults["foo"].Error)
		assert.Len(t, routes.ListenerResults["foo"].Routes, 1)
	})

	t.Run("HTTPRoutes in other namespace for listener", func(t *testing.T) {
		scheme, builder := setup()

		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo",
				Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Namespaces: &gwv1.RouteNamespaces{
						From: ptr.To(gwv1.NamespacesFromAll),
					},
				},
			},
		}

		hr := httpRoute()
		hr.Namespace = "default2"
		hr.Spec.ParentRefs = []gwv1.ParentReference{
			{
				Name:      "test",
				Namespace: ptr.To(gwv1.Namespace("default")),
			},
		}

		fakeClient := builder.WithObjects(hr).Build()
		gq := query.NewData(fakeClient, scheme)

		routes, err := gq.GetRoutesForGateway(ctx, gw)
		assert.NoError(t, err)
		assert.Empty(t, routes.RouteErrors)
		assert.NoError(t, routes.ListenerResults["foo"].Error)
		assert.Len(t, routes.ListenerResults["foo"].Routes, 1)
	})

	t.Run("Listener Does Not Allow Route Kind", func(t *testing.T) {
		scheme, builder := setup()

		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo",
				Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: "FakeKind"}},
				},
			},
			{
				Name:     "bar",
				Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: "FakeKind2"}},
				},
			},
		}

		hr := httpRoute()
		hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
			Name: gwv1.ObjectName(gw.Name),
		})

		fakeClient := builder.WithObjects(hr).Build()
		gq := query.NewData(fakeClient, scheme)

		routes, err := gq.GetRoutesForGateway(ctx, gw)
		assert.NoError(t, err)
		assert.Len(t, routes.RouteErrors, 1)
		assert.Equal(t, query.ErrNotAllowedByListeners, routes.RouteErrors[0].Error.E)
		assert.Equal(t, gwv1.RouteReasonNotAllowedByListeners, routes.RouteErrors[0].Error.Reason)
	})

	t.Run("Should NOT error when one listener allows route", func(t *testing.T) {
		scheme, builder := setup()

		gwWithListener := gw()
		gwWithListener.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo",
				Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: "FakeKind"}},
				},
			},
			{
				Name:     "foo2",
				Protocol: gwv1.HTTPProtocolType,
			},
		}

		hr := httpRoute()
		hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
			Name: gwv1.ObjectName(gwWithListener.Name),
		})

		fakeClient := builder.WithObjects(hr).Build()
		gq := query.NewData(fakeClient, scheme)

		routes, err := gq.GetRoutesForGateway(ctx, gwWithListener)
		assert.NoError(t, err)
		assert.Empty(t, routes.RouteErrors)
		assert.Len(t, routes.ListenerResults["foo2"].Routes, 1)
		assert.NoError(t, routes.ListenerResults["foo2"].Error)
		assert.Empty(t, routes.ListenerResults["foo"].Routes)
		assert.NoError(t, routes.ListenerResults["foo"].Error)
	})

	t.Run("One Listener Allows Route", func(t *testing.T) {
		scheme, builder := setup()

		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo",
				Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: "FakeKind"}},
				},
			},
			{
				Name:     "bar",
				Protocol: gwv1.HTTPProtocolType,
			},
		}

		hr := httpRoute()
		hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
			Name: gwv1.ObjectName(gw.Name),
		})

		fakeClient := builder.WithObjects(hr).Build()
		gq := query.NewData(fakeClient, scheme)

		routes, err := gq.GetRoutesForGateway(ctx, gw)
		assert.NoError(t, err)
		assert.Empty(t, routes.RouteErrors)
		assert.Len(t, routes.ListenerResults["bar"].Routes, 1)
		assert.Empty(t, routes.ListenerResults["foo"].Routes)
	})

	t.Run("Listener Hostnames Do Not Intersect with Route", func(t *testing.T) {
		scheme, builder := setup()

		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo",
				Protocol: gwv1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gwv1.Hostname("foo.com")),
			},
			{
				Name:     "bar",
				Protocol: gwv1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gwv1.Hostname("bar.com")),
			},
		}

		hr := httpRoute()
		hr.Spec.Hostnames = append(hr.Spec.Hostnames, "baz.com")
		hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
			Name: gwv1.ObjectName(gw.Name),
		})

		fakeClient := builder.WithObjects(hr).Build()
		gq := query.NewData(fakeClient, scheme)

		routes, err := gq.GetRoutesForGateway(ctx, gw)
		assert.NoError(t, err)
		assert.Len(t, routes.RouteErrors, 1)
		assert.Equal(t, query.ErrNoMatchingListenerHostname, routes.RouteErrors[0].Error.E)
		assert.Equal(t, gwv1.RouteReasonNoMatchingListenerHostname, routes.RouteErrors[0].Error.Reason)
		assert.Equal(t, hr.Spec.ParentRefs[0], routes.RouteErrors[0].ParentRef)
	})

	t.Run("One Listener Hostname Intersects with Route", func(t *testing.T) {
		scheme, builder := setup()

		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo",
				Protocol: gwv1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gwv1.Hostname("foo.com")),
			},
			{
				Name:     "bar",
				Protocol: gwv1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr.To(gwv1.Hostname("baz.com")),
			},
		}

		hr := httpRoute()
		hr.Spec.Hostnames = append(hr.Spec.Hostnames, "baz.com")
		hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
			Name: gwv1.ObjectName(gw.Name),
		})

		fakeClient := builder.WithObjects(hr).Build()
		gq := query.NewData(fakeClient, scheme)

		routes, err := gq.GetRoutesForGateway(ctx, gw)
		assert.NoError(t, err)
		assert.Empty(t, routes.RouteErrors)
		assert.Len(t, routes.ListenerResults["bar"].Routes, 1)
		assert.Empty(t, routes.ListenerResults["foo"].Routes)
	})

	t.Run("Listener Hostname Intersection", func(t *testing.T) {
		tests := []struct {
			listenerHostname string
			routeHostnames   []string
			expectedMatch    bool
		}{
			{"foo.com", []string{"foo.com"}, true},
			{"*.foo.com", []string{"bar.foo.com", "baz.foo.com"}, true},
			{"bar.com", []string{"foo.com"}, false},
			{"*.example.com", []string{"test.example.com", "another.com"}, true},
			{"baz.com", []string{"qux.com"}, false},
		}

		for _, tt := range tests {
			t.Run(tt.listenerHostname, func(t *testing.T) {
				scheme, builder := setup()

				gw := gw()
				gw.Spec.Listeners = []gwv1.Listener{
					{
						Name:     "foo",
						Protocol: gwv1.HTTPProtocolType,
						Hostname: ptr.To(gwv1.Hostname(tt.listenerHostname)),
					},
				}

				hr := httpRoute()
				for _, hostname := range tt.routeHostnames {
					hr.Spec.Hostnames = append(hr.Spec.Hostnames, gwv1.Hostname(hostname))
				}

				hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
					Name: gwv1.ObjectName(gw.Name),
				})

				fakeClient := builder.WithObjects(hr).Build()
				gq := query.NewData(fakeClient, scheme)

				routes, err := gq.GetRoutesForGateway(ctx, gw)
				assert.NoError(t, err)

				if tt.expectedMatch {
					assert.NotEmpty(t, routes.ListenerResults["foo"].Routes)
				} else {
					assert.Empty(t, routes.ListenerResults["foo"].Routes)
				}
			})
		}
	})

	t.Run("Invalid Label Selector", func(t *testing.T) {
		scheme, builder := setup()

		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo",
				Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Namespaces: &gwv1.RouteNamespaces{
						From:     ptr.To(gwv1.NamespacesFromSelector),
						Selector: nil,
					},
				},
			},
		}

		hr := httpRoute()
		hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
			Name: gwv1.ObjectName(gw.Name),
		})

		fakeClient := builder.WithObjects(hr).Build()
		gq := query.NewData(fakeClient, scheme)

		routes, err := gq.GetRoutesForGateway(ctx, gw)
		assert.NoError(t, err)
		assert.Error(t, routes.ListenerResults["foo"].Error)
		assert.Equal(t, query.ErrMissingSelector, routes.ListenerResults["foo"].Error)
	})

	t.Run("Listeners Don't Match Route", func(t *testing.T) {
		scheme, builder := setup()

		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo",
				Protocol: gwv1.HTTPProtocolType,
				Port:     80,
			},
			{
				Name:     "bar",
				Protocol: gwv1.HTTPProtocolType,
				Port:     81,
			},
		}

		hr := httpRoute()
		var badPort gwv1.PortNumber = 1234
		hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
			Name: gwv1.ObjectName(gw.Name),
			Port: &badPort,
		})

		fakeClient := builder.WithObjects(hr).Build()
		gq := query.NewData(fakeClient, scheme)

		routes, err := gq.GetRoutesForGateway(ctx, gw)
		assert.NoError(t, err)
		assert.Len(t, routes.RouteErrors, 1)
		assert.Equal(t, query.ErrNoMatchingParent, routes.RouteErrors[0].Error.E)
		assert.Equal(t, gwv1.RouteReasonNoMatchingParent, routes.RouteErrors[0].Error.Reason)
		assert.Equal(t, hr.Spec.ParentRefs[0], routes.RouteErrors[0].ParentRef)
	})
}

// Test Helper Functions
func gw() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
	}
}

func svc(namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: namespace,
		},
	}
}

func httpRoute() *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}
}

func refGrant() *gwv1b1.ReferenceGrant {
	return &gwv1b1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default2",
		},
		Spec: gwv1b1.ReferenceGrantSpec{
			From: []gwv1b1.ReferenceGrantFrom{
				{
					Group:     "gateway.networking.k8s.io",
					Kind:      "HTTPRoute",
					Namespace: "default",
				},
			},
			To: []gwv1b1.ReferenceGrantTo{
				{
					Group: "core",
					Kind:  "Service",
				},
			},
		},
	}
}

func secret(ns string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "foo",
		},
	}
}

func refGrantSecret() *gwv1b1.ReferenceGrant {
	return &gwv1b1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default2",
			Name:      "foo",
		},
		Spec: gwv1b1.ReferenceGrantSpec{
			From: []gwv1b1.ReferenceGrantFrom{
				{
					Group:     gwv1.Group("gateway.networking.k8s.io"),
					Kind:      gwv1.Kind("Gateway"),
					Namespace: gwv1.Namespace("default"),
				},
			},
			To: []gwv1b1.ReferenceGrantTo{
				{
					Group: gwv1.Group("core"),
					Kind:  gwv1.Kind("Secret"),
				},
			},
		},
	}
}

func toFrom(scheme *runtime.Scheme, obj client.Object) query.From {
	return query.FromObject{Scheme: scheme, Object: obj}
}
