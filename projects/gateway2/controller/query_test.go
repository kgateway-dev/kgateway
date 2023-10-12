package controller_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gateway2/controller"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	api "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var _ = Describe("Query", func() {

	var (
		scheme  *runtime.Scheme
		builder *fake.ClientBuilder
	)

	BeforeEach(func() {
		scheme = controller.NewScheme()
		builder = fake.NewClientBuilder().WithScheme(scheme)
		controller.IterateIndices(func(o client.Object, f string, fun client.IndexerFunc) error {
			builder.WithIndex(o, f, fun)
			return nil
		})
	})
	Describe("GetBackendForRef", func() {
		It("should get service from same namespace", func() {
			fakeClient := fake.NewFakeClient(svc("default"))

			gq := controller.NewData(fakeClient, controller.NewScheme())
			ref := &api.HTTPBackendRef{
				BackendRef: api.BackendRef{
					BackendObjectReference: api.BackendObjectReference{
						Name: "foo",
					},
				},
			}
			backend, err := gq.GetBackendForRef(context.Background(), httpRoute(), ref)
			Expect(err).NotTo(HaveOccurred())
			Expect(backend).NotTo(BeNil())
			Expect(backend.GetName()).To(Equal("foo"))
			Expect(backend.GetNamespace()).To(Equal("default"))
		})

		It("should get service from different ns if we have a ref grant", func() {
			rg := refGrant()
			fakeClient := builder.WithObjects(svc("default2"), rg).Build()
			gq := controller.NewData(fakeClient, scheme)
			ref := &api.HTTPBackendRef{
				BackendRef: api.BackendRef{
					BackendObjectReference: api.BackendObjectReference{
						Name:      "foo",
						Namespace: nsptr("default2"),
					},
				},
			}
			backend, err := gq.GetBackendForRef(context.Background(), httpRoute(), ref)
			Expect(err).NotTo(HaveOccurred())
			Expect(backend).NotTo(BeNil())
			Expect(backend.GetName()).To(Equal("foo"))
			Expect(backend.GetNamespace()).To(Equal("default2"))
		})

		It("should fail with service not found if we have a ref grant", func() {
			rg := refGrant()
			fakeClient := builder.WithObjects(rg).Build()
			gq := controller.NewData(fakeClient, scheme)
			ref := &api.HTTPBackendRef{
				BackendRef: api.BackendRef{
					BackendObjectReference: api.BackendObjectReference{
						Name:      "foo",
						Namespace: nsptr("default2"),
					},
				},
			}
			backend, err := gq.GetBackendForRef(context.Background(), httpRoute(), ref)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			Expect(backend).To(BeNil())
		})

		It("should fail getting a service with ref grant with wrong from", func() {
			ref := &api.HTTPBackendRef{
				BackendRef: api.BackendRef{
					BackendObjectReference: api.BackendObjectReference{
						Name:      "foo",
						Namespace: nsptr("default2"),
					},
				},
			}
			rg := &api.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default2",
					Name:      "foo",
				},
				Spec: api.ReferenceGrantSpec{
					From: []api.ReferenceGrantFrom{
						{
							Group:     api.Group("gateway.networking.k8s.io"),
							Kind:      api.Kind("NotGateway"),
							Namespace: api.Namespace("default"),
						},
						{
							Group:     api.Group("gateway.networking.k8s.io"),
							Kind:      api.Kind("Gateway"),
							Namespace: api.Namespace("default2"),
						},
					},
					To: []api.ReferenceGrantTo{
						{
							Group: api.Group("core"),
							Kind:  api.Kind("Service"),
						},
					},
				},
			}
			fakeClient := builder.WithObjects(rg, svc("default2")).Build()

			gq := controller.NewData(fakeClient, scheme)
			backend, err := gq.GetBackendForRef(context.Background(), httpRoute(), ref)
			Expect(err).To(MatchError(controller.ErrMissingReferenceGrant))
			Expect(backend).To(BeNil())
		})

		It("should fail getting a service with no ref grant", func() {
			fakeClient := builder.WithObjects(svc("default3")).Build()
			gq := controller.NewData(fakeClient, scheme)
			ref := &api.HTTPBackendRef{
				BackendRef: api.BackendRef{
					BackendObjectReference: api.BackendObjectReference{
						Name:      "foo",
						Namespace: nsptr("default3"),
					},
				},
			}
			backend, err := gq.GetBackendForRef(context.Background(), httpRoute(), ref)
			Expect(err).To(MatchError(controller.ErrMissingReferenceGrant))
			Expect(backend).To(BeNil())
		})

		It("should fail getting a service with wrong ref grant", func() {
			rg := refGrant()
			fakeClient := builder.WithObjects(svc("default3"), rg).Build()

			gq := controller.NewData(fakeClient, scheme)
			ref := &api.HTTPBackendRef{
				BackendRef: api.BackendRef{
					BackendObjectReference: api.BackendObjectReference{
						Name:      "foo",
						Namespace: nsptr("default3"),
					},
				},
			}
			backend, err := gq.GetBackendForRef(context.Background(), httpRoute(), ref)
			Expect(err).To(MatchError(controller.ErrMissingReferenceGrant))
			Expect(backend).To(BeNil())
		})
	})

	Describe("GetSecretRef", func() {

		It("should get secret from different ns if we have a ref grant", func() {
			rg := refGrantSecret()
			fakeClient := builder.WithObjects(secret("default2"), rg).Build()
			gq := controller.NewData(fakeClient, scheme)
			ref := &api.SecretObjectReference{
				Name:      "foo",
				Namespace: nsptr("default2"),
			}
			backend, err := gq.GetSecretForRef(context.Background(), gw(), ref)
			Expect(err).NotTo(HaveOccurred())
			Expect(backend).NotTo(BeNil())
			Expect(backend.GetName()).To(Equal("foo"))
			Expect(backend.GetNamespace()).To(Equal("default2"))
		})
	})

	Describe("Get Routes", func() {

		It("should get http routes for listener", func() {
			gwWithListener := gw()
			gwWithListener.Spec.Listeners = []api.Listener{
				{
					Name:     "foo",
					Protocol: api.HTTPProtocolType,
				},
			}
			hr := httpRoute()
			hr.Spec.ParentRefs = []api.ParentReference{
				{
					Name: "test",
				},
			}

			fakeClient := builder.WithObjects(hr).Build()
			gq := controller.NewData(fakeClient, scheme)
			routes, err := gq.GetRoutesForGw(context.Background(), gwWithListener)

			Expect(err).NotTo(HaveOccurred())
			Expect(routes).NotTo(BeNil())
			Expect(len(routes["foo"])).To(Equal(1))
		})

		It("should get http routes in other ns for listener", func() {
			gwWithListener := gw()
			all := api.NamespacesFromAll
			gwWithListener.Spec.Listeners = []api.Listener{
				{
					Name:     "foo",
					Protocol: api.HTTPProtocolType,
					AllowedRoutes: &api.AllowedRoutes{
						Namespaces: &api.RouteNamespaces{
							From: &all,
						},
					},
				},
			}
			hr := httpRoute()
			hr.Namespace = "default2"
			hr.Spec.ParentRefs = []api.ParentReference{
				{
					Name: "test",
                    Namespace: nsptr("default"),
				},
			}

			fakeClient := builder.WithObjects(hr).Build()
			gq := controller.NewData(fakeClient, scheme)
			routes, err := gq.GetRoutesForGw(context.Background(), gwWithListener)

			Expect(err).NotTo(HaveOccurred())
			Expect(routes).NotTo(BeNil())
			Expect(len(routes["foo"])).To(Equal(1))
		})
	})
})

func refGrantSecret() *api.ReferenceGrant {
	return &api.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default2",
			Name:      "foo",
		},
		Spec: api.ReferenceGrantSpec{
			From: []api.ReferenceGrantFrom{
				{
					Group:     api.Group("gateway.networking.k8s.io"),
					Kind:      api.Kind("Gateway"),
					Namespace: api.Namespace("default"),
				},
			},
			To: []api.ReferenceGrantTo{
				{
					Group: api.Group("core"),
					Kind:  api.Kind("Secret"),
				},
			},
		},
	}
}
func refGrant() *api.ReferenceGrant {
	return &api.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default2",
			Name:      "foo",
		},
		Spec: api.ReferenceGrantSpec{
			From: []api.ReferenceGrantFrom{
				{
					Group:     api.Group("gateway.networking.k8s.io"),
					Kind:      api.Kind("HTTPRoute"),
					Namespace: api.Namespace("default"),
				},
			},
			To: []api.ReferenceGrantTo{
				{
					Group: api.Group("core"),
					Kind:  api.Kind("Service"),
				},
			},
		},
	}
}

func httpRoute() *api.HTTPRoute {
	return &api.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
	}

}

func gw() *api.Gateway {
	return &api.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
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

func svc(ns string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "foo",
		},
	}
}

func nsptr(s string) *api.Namespace {
	var ns api.Namespace = api.Namespace(s)
	return &ns
}
