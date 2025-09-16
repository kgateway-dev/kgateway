package query_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	"istio.io/istio/pkg/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
)

//go:generate go tool mockgen -destination mocks/mock_queries.go -package mocks github.com/kgateway-dev/kgateway/v2/internal/kgateway/query GatewayQueries

var _ = Describe("Query", func() {
	Describe("GetSecretRef", func() {
		It("should get secret from different ns if we have a ref grant", func() {
			rg := refGrantSecret()
			gq := newQueries(GinkgoT(), secret("default2"), rg)
			ref := gwv1.SecretObjectReference{
				Name:      "foo",
				Namespace: nsptr("default2"),
			}
			fromGk := schema.GroupKind{
				Group: gwv1.GroupName,
				Kind:  "Gateway",
			}
			backend, err := gq.GetSecretForRef(krt.TestingDummyContext{}, context.Background(), fromGk, "default", ref)
			Expect(err).NotTo(HaveOccurred())
			Expect(backend).NotTo(BeNil())
			Expect(backend.GetName()).To(Equal("foo"))
			Expect(backend.GetNamespace()).To(Equal("default2"))
		})
	})

	Describe("Get Routes", func() {
		It("should get http routes for listener", func() {
			gwWithListener := gw()
			gwWithListener.Spec.Listeners = []gwv1.Listener{
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

			gq := newQueries(GinkgoT(), hr)
			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gwWithListener})

			Expect(err).NotTo(HaveOccurred())
			Expect(routes.RouteErrors).To(BeEmpty())
			Expect(routes.GetListenerResult(gwWithListener, "foo").Error).NotTo(HaveOccurred())
			Expect(routes.GetListenerResult(gwWithListener, "foo").Routes).To(HaveLen(1))
		})

		It("should get http routes in other ns for listener", func() {
			gwWithListener := gw()
			all := gwv1.NamespacesFromAll
			gwWithListener.Spec.Listeners = []gwv1.Listener{
				{
					Name:     "foo",
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Namespaces: &gwv1.RouteNamespaces{
							From: &all,
						},
					},
				},
			}
			hr := httpRoute()
			hr.Namespace = "default2"
			hr.Spec.ParentRefs = []gwv1.ParentReference{
				{
					Name:      "test",
					Namespace: nsptr("default"),
				},
			}

			gq := newQueries(GinkgoT(), hr)

			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gwWithListener})

			Expect(err).NotTo(HaveOccurred())
			Expect(routes.RouteErrors).To(BeEmpty())
			Expect(routes.GetListenerResult(gwWithListener, "foo").Error).NotTo(HaveOccurred())
			Expect(routes.GetListenerResult(gwWithListener, "foo").Routes).To(HaveLen(1))
		})

		It("should ignore http routes for wrong kind", func() {
			gwWithListener := gw()
			gwWithListener.Spec.Listeners = []gwv1.Listener{
				{
					Name:     "foo",
					Protocol: gwv1.HTTPProtocolType,
				},
			}
			hr := httpRoute()
			hr.Spec.ParentRefs = []gwv1.ParentReference{
				{
					Name:  "test",
					Group: ptr.To(gwv1.Group("")),
					Kind:  ptr.To(gwv1.Kind("Service")),
				},
			}

			gq := newQueries(GinkgoT(), hr)
			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gwWithListener})

			Expect(err).NotTo(HaveOccurred())
			Expect(routes.RouteErrors).To(BeEmpty())
		})

		It("should error with invalid label selector", func() {
			gwWithListener := gw()
			selector := gwv1.NamespacesFromSelector
			gwWithListener.Spec.Listeners = []gwv1.Listener{
				{
					Name:     "foo",
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Namespaces: &gwv1.RouteNamespaces{
							From:     &selector,
							Selector: nil,
						},
					},
				},
			}
			hr := httpRoute()
			hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
				Name: gwv1.ObjectName(gwWithListener.Name),
			})

			gq := newQueries(GinkgoT(), hr)
			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gwWithListener})

			Expect(err).NotTo(HaveOccurred())
			Expect(routes.GetListenerResult(gwWithListener, "foo").Error).To(MatchError("selector must be set"))
		})

		It("should error when listeners do not allow route", func() {
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
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{{Kind: "FakeKind2"}},
					},
				},
			}
			hr := httpRoute()
			hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
				Name: gwv1.ObjectName(gwWithListener.Name),
			})

			gq := newQueries(GinkgoT(), hr)
			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gwWithListener})

			Expect(err).NotTo(HaveOccurred())
			Expect(routes.RouteErrors[0].Error.E).To(MatchError(query.ErrNotAllowedByListeners))
			Expect(routes.RouteErrors[0].Error.Reason).To(Equal(gwv1.RouteReasonNotAllowedByListeners))
			Expect(routes.RouteErrors[0].ParentRef).To(Equal(hr.Spec.ParentRefs[0]))
		})

		It("should NOT error when one listener allows route", func() {
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

			gq := newQueries(GinkgoT(), hr)
			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gwWithListener})

			Expect(err).NotTo(HaveOccurred())
			Expect(routes.RouteErrors).To(BeEmpty())
			Expect(routes.GetListenerResult(gwWithListener, "foo2").Routes).To(HaveLen(1))
			Expect(routes.GetListenerResult(gwWithListener, "foo2").Error).NotTo(HaveOccurred())
			Expect(routes.GetListenerResult(gwWithListener, "foo").Routes).To(BeEmpty())
			Expect(routes.GetListenerResult(gwWithListener, "foo").Error).NotTo(HaveOccurred())
		})

		It("should error when listeners don't match route", func() {
			gwWithListener := gw()
			gwWithListener.Spec.Listeners = []gwv1.Listener{
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
			var port gwv1.PortNumber = 1234
			hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
				Name: gwv1.ObjectName(gwWithListener.Name),
				Port: &port,
			})

			gq := newQueries(GinkgoT(), hr)
			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gwWithListener})

			Expect(err).NotTo(HaveOccurred())
			Expect(routes.RouteErrors[0].Error.E).To(MatchError(query.ErrNoMatchingParent))
			Expect(routes.RouteErrors[0].Error.Reason).To(Equal(gwv1.RouteReasonNoMatchingParent))
			Expect(routes.RouteErrors[0].ParentRef).To(Equal(hr.Spec.ParentRefs[0]))
		})

		It("should NOT error when one listener match route", func() {
			gwWithListener := gw()
			gwWithListener.Spec.Listeners = []gwv1.Listener{
				{
					Name:     "foo",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
				},
				{
					Name:     "foo2",
					Protocol: gwv1.HTTPProtocolType,
					Port:     81,
				},
			}
			hr := httpRoute()
			var port gwv1.PortNumber = 81
			hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
				Name: gwv1.ObjectName(gwWithListener.Name),
				Port: &port,
			})

			gq := newQueries(GinkgoT(), hr)
			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gwWithListener})

			Expect(err).NotTo(HaveOccurred())
			Expect(routes.RouteErrors).To(BeEmpty())
			Expect(routes.GetListenerResult(gwWithListener, "foo2").Routes).To(HaveLen(1))
			Expect(routes.GetListenerResult(gwWithListener, "foo").Routes).To(BeEmpty())
		})

		It("should error when listeners hostnames don't intersect", func() {
			gwWithListener := gw()
			var hostname gwv1.Hostname = "foo.com"
			var hostname2 gwv1.Hostname = "foo2.com"
			gwWithListener.Spec.Listeners = []gwv1.Listener{
				{
					Name:     "foo",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
					Hostname: &hostname,
				},
				{
					Name:     "foo2",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
					Hostname: &hostname2,
				},
			}
			hr := httpRoute()
			hr.Spec.Hostnames = append(hr.Spec.Hostnames, "bar.com")
			hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
				Name: gwv1.ObjectName(gwWithListener.Name),
			})

			gq := newQueries(GinkgoT(), hr)
			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gwWithListener})

			Expect(err).NotTo(HaveOccurred())
			Expect(routes.RouteErrors[0].Error.E).To(MatchError(query.ErrNoMatchingListenerHostname))
			Expect(routes.RouteErrors[0].Error.Reason).To(Equal(gwv1.RouteReasonNoMatchingListenerHostname))
			Expect(routes.RouteErrors[0].ParentRef).To(Equal(hr.Spec.ParentRefs[0]))
		})

		It("should NOT error when one listener hostname do intersect", func() {
			gwWithListener := gw()
			var hostname gwv1.Hostname = "foo.com"
			var hostname2 gwv1.Hostname = "bar.com"
			gwWithListener.Spec.Listeners = []gwv1.Listener{
				{
					Name:     "foo",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
					Hostname: &hostname,
				},
				{
					Name:     "foo2",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
					Hostname: &hostname2,
				},
			}
			hr := httpRoute()
			hr.Spec.Hostnames = append(hr.Spec.Hostnames, "bar.com")
			hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
				Name: gwv1.ObjectName(gwWithListener.Name),
			})

			gq := newQueries(GinkgoT(), hr)
			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gwWithListener})

			Expect(err).NotTo(HaveOccurred())
			Expect(routes.RouteErrors).To(BeEmpty())
			Expect(routes.GetListenerResult(gwWithListener, "foo2").Routes).To(HaveLen(1))
			Expect(routes.GetListenerResult(gwWithListener, "foo").Routes).To(BeEmpty())
		})

		It("should error for one parent ref but not the other", func() {
			gwWithListener := gw()
			var hostname gwv1.Hostname = "foo.com"
			gwWithListener.Spec.Listeners = []gwv1.Listener{
				{
					Name:     "foo",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
					Hostname: &hostname,
				},
			}
			hr := httpRoute()
			var badPort gwv1.PortNumber = 81
			hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
				Name: gwv1.ObjectName(gwWithListener.Name),
				Port: &badPort,
			}, gwv1.ParentReference{
				Name: gwv1.ObjectName(gwWithListener.Name),
			})

			gq := newQueries(GinkgoT(), hr)
			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gwWithListener})

			Expect(err).NotTo(HaveOccurred())
			Expect(routes.RouteErrors).To(HaveLen(1))
			Expect(routes.GetListenerResult(gwWithListener, "foo").Routes).To(HaveLen(1))
			Expect(routes.GetListenerResult(gwWithListener, "foo").Routes[0].ParentRef).To(Equal(gwv1.ParentReference{
				Name: hr.Spec.ParentRefs[1].Name,
			}))
			Expect(routes.RouteErrors[0].Error.E).To(MatchError(query.ErrNoMatchingParent))
			Expect(routes.RouteErrors[0].Error.Reason).To(Equal(gwv1.RouteReasonNoMatchingParent))
			Expect(routes.RouteErrors[0].ParentRef).To(Equal(hr.Spec.ParentRefs[0]))
		})

		Context("test host intersection", func() {
			expectHostnamesToMatch := func(lh string, rh []string, expectedHostnames ...string) {
				gwWithListener := gw()
				gwWithListener.Spec.Listeners = []gwv1.Listener{
					{
						Name:     "foo",
						Protocol: gwv1.HTTPProtocolType,
					},
				}
				if lh != "" {
					h := gwv1.Hostname(lh)
					gwWithListener.Spec.Listeners[0].Hostname = &h

				}

				hr := httpRoute()
				for _, h := range rh {
					hr.Spec.Hostnames = append(hr.Spec.Hostnames, gwv1.Hostname(h))
				}
				hr.Spec.ParentRefs = append(hr.Spec.ParentRefs, gwv1.ParentReference{
					Name: gwv1.ObjectName(gwWithListener.Name),
				})

				gq := newQueries(GinkgoT(), hr)
				routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gwWithListener})

				Expect(err).NotTo(HaveOccurred())
				if expectedHostnames == nil {
					expectedHostnames = []string{}
				}
				Expect(routes.GetListenerResult(gwWithListener, "foo").Routes[0].Hostnames()).To(Equal(expectedHostnames))
			}

			It("should work with identical names", func() {
				expectHostnamesToMatch("foo.com", []string{"foo.com"}, "foo.com")
			})
			It("should work with specific listeners and prefix http", func() {
				expectHostnamesToMatch("bar.foo.com", []string{"*.foo.com", "foo.com", "example.com"}, "bar.foo.com")
			})
			It("should work with prefix listeners and specific http", func() {
				expectHostnamesToMatch("*.foo.com", []string{"bar.foo.com", "foo.com", "far.foo.com", "blah.com"}, "bar.foo.com", "far.foo.com")
			})
			It("should work with catch all listener hostname", func() {
				expectHostnamesToMatch("", []string{"foo.com", "blah.com"}, "foo.com", "blah.com")
			})
			It("should work with catch all http hostname", func() {
				expectHostnamesToMatch("foo.com", nil, "foo.com")
			})
			It("should work with listener prefix and catch all http hostname", func() {
				expectHostnamesToMatch("*.foo.com", nil, "*.foo.com")
			})
			It("should work with double catch all", func() {
				expectHostnamesToMatch("", nil)
			})
		})

		It("should match TCPRoutes for Listener", func() {
			gw := gw()
			gw.Spec.Listeners = []gwv1.Listener{
				{
					Name:     "foo-tcp",
					Protocol: gwv1.TCPProtocolType,
				},
			}

			tcpRoute := tcpRoute("test-tcp-route", gw.Namespace)
			tcpRoute.Spec = gwv1a2.TCPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name: gwv1.ObjectName(gw.Name),
						},
					},
				},
			}

			gq := newQueries(GinkgoT(), tcpRoute)
			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})

			Expect(err).NotTo(HaveOccurred())
			Expect(routes.GetListenerResult(gw, string(gw.Spec.Listeners[0].Name)).Routes).To(HaveLen(1))
			Expect(routes.GetListenerResult(gw, string(gw.Spec.Listeners[0].Name)).Error).NotTo(HaveOccurred())
		})

		It("should get TCPRoutes in other namespace for listener", func() {
			gw := gw()
			gw.Spec.Listeners = []gwv1.Listener{
				{
					Name:     "foo-tcp",
					Protocol: gwv1.TCPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Namespaces: &gwv1.RouteNamespaces{
							From: ptr.To(gwv1.NamespacesFromAll),
						},
					},
				},
			}

			tcpRoute := tcpRoute("test-tcp-route", "other-ns")
			tcpRoute.Spec = gwv1a2.TCPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name:      gwv1.ObjectName(gw.Name),
							Namespace: ptr.To(gwv1.Namespace(gw.Namespace)),
						},
					},
				},
			}

			gq := newQueries(GinkgoT(), tcpRoute)

			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})

			Expect(err).NotTo(HaveOccurred())
			Expect(routes.GetListenerResult(gw, "foo-tcp").Error).NotTo(HaveOccurred())
			Expect(routes.GetListenerResult(gw, "foo-tcp").Routes).To(HaveLen(1))
		})

		It("should error when listeners don't match TCPRoute", func() {
			gw := gw()
			gw.Spec.Listeners = []gwv1.Listener{
				{
					Name:     "foo-tcp",
					Protocol: gwv1.TCPProtocolType,
					Port:     8080,
				},
				{
					Name:     "bar-tcp",
					Protocol: gwv1.TCPProtocolType,
					Port:     8081,
				},
			}

			tcpRoute := tcpRoute("test-tcp-route", gw.Namespace)
			var badPort gwv1.PortNumber = 9999
			tcpRoute.Spec = gwv1a2.TCPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name: gwv1.ObjectName(gw.Name),
							Port: &badPort,
						},
					},
				},
			}

			gq := newQueries(GinkgoT(), tcpRoute)
			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})

			Expect(err).NotTo(HaveOccurred())
			Expect(routes.RouteErrors).To(HaveLen(1))
			Expect(routes.RouteErrors[0].Error.E).To(MatchError(query.ErrNoMatchingParent))
			Expect(routes.RouteErrors[0].Error.Reason).To(Equal(gwv1.RouteReasonNoMatchingParent))
			Expect(routes.RouteErrors[0].ParentRef).To(Equal(tcpRoute.Spec.ParentRefs[0]))
		})

		It("should error when listener does not allow TCPRoute kind", func() {
			gw := gw()
			gw.Spec.Listeners = []gwv1.Listener{
				{
					Name:     "foo-tcp",
					Protocol: gwv1.TCPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{{Kind: "FakeKind"}},
					},
				},
			}

			tcpRoute := tcpRoute("test-tcp-route", gw.Namespace)
			tcpRoute.Spec = gwv1a2.TCPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name: gwv1.ObjectName(gw.Name),
						},
					},
				},
			}

			gq := newQueries(GinkgoT(), tcpRoute)

			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})
			Expect(err).NotTo(HaveOccurred())
			Expect(routes.RouteErrors).To(HaveLen(1))
			Expect(routes.RouteErrors[0].Error.E).To(MatchError(query.ErrNotAllowedByListeners))
		})

		It("should allow TCPRoute for one listener", func() {
			gw := gw()
			gw.Spec.Listeners = []gwv1.Listener{
				{
					Name:     "foo-tcp",
					Protocol: gwv1.TCPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{{Kind: wellknown.TCPRouteKind}},
					},
				},
				{
					Name:     "bar",
					Protocol: gwv1.TCPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{{Kind: "FakeKind"}},
					},
				},
			}

			tcpRoute := tcpRoute("test-tcp-route", gw.Namespace)
			tcpRoute.Spec = gwv1a2.TCPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name: gwv1.ObjectName(gw.Name),
						},
					},
				},
			}

			gq := newQueries(GinkgoT(), tcpRoute)

			routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})
			Expect(err).NotTo(HaveOccurred())
			Expect(routes.RouteErrors).To(BeEmpty())
			Expect(routes.GetListenerResult(gw, "foo-tcp").Routes).To(HaveLen(1))
			Expect(routes.GetListenerResult(gw, "bar").Routes).To(BeEmpty())
		})
	})

	It("should match TLSRoutes for Listener", func() {
		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo-tls",
				Protocol: gwv1.TLSProtocolType,
			},
		}

		tlsRoute := &gwv1a2.TLSRoute{
			TypeMeta: metav1.TypeMeta{
				Kind:       wellknown.TLSRouteKind,
				APIVersion: gwv1a2.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-tls-route",
				Namespace: gw.Namespace,
			},
			Spec: gwv1a2.TLSRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name: gwv1.ObjectName(gw.Name),
						},
					},
				},
			},
		}

		gq := newQueries(GinkgoT(), tlsRoute)
		routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})

		Expect(err).NotTo(HaveOccurred())
		Expect(routes.GetListenerResult(gw, string(gw.Spec.Listeners[0].Name)).Routes).To(HaveLen(1))
		Expect(routes.GetListenerResult(gw, string(gw.Spec.Listeners[0].Name)).Error).NotTo(HaveOccurred())
	})

	It("should get TLSRoutes in other namespace for listener", func() {
		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo-tls",
				Protocol: gwv1.TLSProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Namespaces: &gwv1.RouteNamespaces{
						From: ptr.To(gwv1.NamespacesFromAll),
					},
				},
			},
		}

		tlsRoute := tlsRoute("test-tls-route", "other-ns")
		tlsRoute.Spec = gwv1a2.TLSRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name:      gwv1.ObjectName(gw.Name),
						Namespace: ptr.To(gwv1.Namespace(gw.Namespace)),
					},
				},
			},
		}

		gq := newQueries(GinkgoT(), tlsRoute)
		routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})

		Expect(err).NotTo(HaveOccurred())
		Expect(routes.GetListenerResult(gw, "foo-tls").Error).NotTo(HaveOccurred())
		Expect(routes.GetListenerResult(gw, "foo-tls").Routes).To(HaveLen(1))
	})

	It("should error when listeners don't match TLSRoute", func() {
		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo-tls",
				Protocol: gwv1.TLSProtocolType,
				Port:     8080,
			},
			{
				Name:     "bar-tls",
				Protocol: gwv1.TLSProtocolType,
				Port:     8081,
			},
		}

		tlsRoute := tlsRoute("test-tls-route", gw.Namespace)
		var badPort gwv1.PortNumber = 9999
		tlsRoute.Spec = gwv1a2.TLSRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: gwv1.ObjectName(gw.Name),
						Port: &badPort,
					},
				},
			},
		}

		gq := newQueries(GinkgoT(), tlsRoute)
		routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})

		Expect(err).NotTo(HaveOccurred())
		Expect(routes.RouteErrors).To(HaveLen(1))
		Expect(routes.RouteErrors[0].Error.E).To(MatchError(query.ErrNoMatchingParent))
		Expect(routes.RouteErrors[0].Error.Reason).To(Equal(gwv1.RouteReasonNoMatchingParent))
		Expect(routes.RouteErrors[0].ParentRef).To(Equal(tlsRoute.Spec.ParentRefs[0]))
	})

	It("should error when listener does not allow TLSRoute kind", func() {
		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo-tls",
				Protocol: gwv1.TLSProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: "FakeKind"}},
				},
			},
		}

		tlsRoute := tlsRoute("test-tls-route", gw.Namespace)
		tlsRoute.Spec = gwv1a2.TLSRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: gwv1.ObjectName(gw.Name),
					},
				},
			},
		}

		gq := newQueries(GinkgoT(), tlsRoute)
		routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})

		Expect(err).NotTo(HaveOccurred())
		Expect(routes.RouteErrors).To(HaveLen(1))
		Expect(routes.RouteErrors[0].Error.E).To(MatchError(query.ErrNotAllowedByListeners))
	})

	It("should allow TLSRoute for one listener", func() {
		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo-tls",
				Protocol: gwv1.TLSProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: wellknown.TLSRouteKind}},
				},
			},
			{
				Name:     "bar",
				Protocol: gwv1.TLSProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: "FakeKind"}},
				},
			},
		}

		tlsRoute := tlsRoute("test-tls-route", gw.Namespace)
		tlsRoute.Spec = gwv1a2.TLSRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: gwv1.ObjectName(gw.Name),
					},
				},
			},
		}

		gq := newQueries(GinkgoT(), tlsRoute)
		routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})

		Expect(err).NotTo(HaveOccurred())
		Expect(routes.RouteErrors).To(BeEmpty())
		Expect(routes.GetListenerResult(gw, "foo-tls").Routes).To(HaveLen(1))
		Expect(routes.GetListenerResult(gw, "bar").Routes).To(BeEmpty())
	})

	// GRPCRoute Tests
	It("should match GRPCRoutes for Listener", func() {
		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo-grpc",
				Protocol: gwv1.HTTPProtocolType, // GRPCRoute attaches to HTTP/HTTPS listeners
			},
		}

		gr := grpcRoute("test-grpc-route", gw.Namespace)
		gr.Spec = gwv1.GRPCRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: gwv1.ObjectName(gw.Name),
					},
				},
			},
		}

		gq := newQueries(GinkgoT(), gr)
		routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})

		Expect(err).NotTo(HaveOccurred())
		Expect(routes.GetListenerResult(gw, string(gw.Spec.Listeners[0].Name)).Routes).To(HaveLen(1))
		Expect(routes.GetListenerResult(gw, string(gw.Spec.Listeners[0].Name)).Error).NotTo(HaveOccurred())
	})

	It("should get GRPCRoutes in other namespace for listener", func() {
		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo-grpc",
				Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Namespaces: &gwv1.RouteNamespaces{
						From: ptr.To(gwv1.NamespacesFromAll),
					},
				},
			},
		}

		gr := grpcRoute("test-grpc-route", "other-ns")
		gr.Spec = gwv1.GRPCRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name:      gwv1.ObjectName(gw.Name),
						Namespace: ptr.To(gwv1.Namespace(gw.Namespace)),
					},
				},
			},
		}

		gq := newQueries(GinkgoT(), gr)
		routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})

		Expect(err).NotTo(HaveOccurred())
		Expect(routes.GetListenerResult(gw, "foo-grpc").Error).NotTo(HaveOccurred())
		Expect(routes.GetListenerResult(gw, "foo-grpc").Routes).To(HaveLen(1))
	})

	It("should error when listeners don't match GRPCRoute", func() {
		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo-grpc",
				Protocol: gwv1.HTTPProtocolType,
				Port:     8080,
			},
			{
				Name:     "bar-grpc",
				Protocol: gwv1.HTTPProtocolType,
				Port:     8081,
			},
		}

		gr := grpcRoute("test-grpc-route", gw.Namespace)
		var badPort gwv1.PortNumber = 9999
		gr.Spec = gwv1.GRPCRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: gwv1.ObjectName(gw.Name),
						Port: &badPort,
					},
				},
			},
		}

		gq := newQueries(GinkgoT(), gr)
		routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})

		Expect(err).NotTo(HaveOccurred())
		Expect(routes.RouteErrors).To(HaveLen(1))
		Expect(routes.RouteErrors[0].Error.E).To(MatchError(query.ErrNoMatchingParent))
		Expect(routes.RouteErrors[0].Error.Reason).To(Equal(gwv1.RouteReasonNoMatchingParent))
		Expect(routes.RouteErrors[0].ParentRef).To(Equal(gr.Spec.ParentRefs[0]))
	})

	It("should error when listener does not allow GRPCRoute kind", func() {
		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo-grpc",
				Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: "FakeKind"}},
				},
			},
		}

		gr := grpcRoute("test-grpc-route", gw.Namespace)
		gr.Spec = gwv1.GRPCRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: gwv1.ObjectName(gw.Name),
					},
				},
			},
		}

		gq := newQueries(GinkgoT(), gr)
		routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})

		Expect(err).NotTo(HaveOccurred())
		Expect(routes.RouteErrors).To(HaveLen(1))
		Expect(routes.RouteErrors[0].Error.E).To(MatchError(query.ErrNotAllowedByListeners))
	})

	It("should allow GRPCRoute for one listener", func() {
		gw := gw()
		gw.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo-grpc",
				Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: wellknown.GRPCRouteKind}},
				},
			},
			{
				Name:     "bar",
				Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: "FakeKind"}},
				},
			},
		}

		gr := grpcRoute("test-grpc-route", gw.Namespace)
		gr.Spec = gwv1.GRPCRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: gwv1.ObjectName(gw.Name),
					},
				},
			},
		}

		gq := newQueries(GinkgoT(), gr)
		routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &ir.Gateway{Obj: gw})

		Expect(err).NotTo(HaveOccurred())
		Expect(routes.RouteErrors).To(BeEmpty())
		Expect(routes.GetListenerResult(gw, "foo-grpc").Routes).To(HaveLen(1))
		Expect(routes.GetListenerResult(gw, "bar").Routes).To(BeEmpty())
	})

	It("should get http routes for a consolidated gateway", func() {
		gwWithListener := gw()
		gwWithListener.Spec.Listeners = []gwv1.Listener{
			{
				Name:     "foo",
				Protocol: gwv1.HTTPProtocolType,
			},
		}
		allNamespaces := gwv1.NamespacesFromAll
		gwWithListener.Spec.AllowedListeners = &gwv1.AllowedListeners{
			Namespaces: &gwv1.ListenerNamespaces{
				From: &allNamespaces,
			},
		}

		lsWithListener := ls()
		gwHR := httpRoute()
		gwHR.Spec.ParentRefs = []gwv1.ParentReference{
			{
				Name: "test",
			},
		}

		lsHR := httpRoute()
		lsHR.Name = "ls-route"
		lsKind := gwv1.Kind(wellknown.XListenerSetKind)
		lsGroup := gwv1.Group(wellknown.XListenerSetGroup)
		lsHR.Spec.ParentRefs = []gwv1.ParentReference{
			{
				Kind:  &lsKind,
				Group: &lsGroup,
				Name:  "ls",
			},
		}

		irGW := ir.Gateway{
			Obj:                 gwWithListener,
			AllowedListenerSets: []ir.ListenerSet{{Obj: lsWithListener}},
		}

		gq := newQueries(GinkgoT(), gwHR, lsHR)

		routes, err := gq.GetRoutesForGateway(krt.TestingDummyContext{}, context.Background(), &irGW)
		Expect(err).NotTo(HaveOccurred())
		Expect(routes.RouteErrors).To(BeEmpty())
		Expect(routes.GetListenerResult(gwWithListener, "foo").Error).NotTo(HaveOccurred())
		Expect(routes.GetListenerResult(gwWithListener, "foo").Routes).To(HaveLen(1))
		Expect(routes.GetListenerResult(lsWithListener, "bar").Error).NotTo(HaveOccurred())
		Expect(routes.GetListenerResult(lsWithListener, "bar").Routes).To(HaveLen(1))
		Expect(routes.GetListenerResult(lsWithListener, string(lsWithListener.Spec.Listeners[0].Name)).Routes[0].GetName()).To(Equal("ls-route"))
	})
})

func refGrantSecret() *gwv1beta1.ReferenceGrant {
	return &gwv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default2",
			Name:      "foo",
		},
		Spec: gwv1beta1.ReferenceGrantSpec{
			From: []gwv1beta1.ReferenceGrantFrom{
				{
					Group:     gwv1.Group("gateway.networking.k8s.io"),
					Kind:      gwv1.Kind("Gateway"),
					Namespace: gwv1.Namespace("default"),
				},
			},
			To: []gwv1beta1.ReferenceGrantTo{
				{
					Group: gwv1.Group("core"),
					Kind:  gwv1.Kind("Secret"),
				},
			},
		},
	}
}

func httpRoute() *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       wellknown.HTTPRouteKind,
			APIVersion: gwv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
	}
}

func gw() *gwv1.Gateway {
	return &gwv1.Gateway{
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

func tcpRoute(name, ns string) *gwv1a2.TCPRoute {
	return &gwv1a2.TCPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       wellknown.TCPRouteKind,
			APIVersion: gwv1a2.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

func tlsRoute(name, ns string) *gwv1a2.TLSRoute {
	return &gwv1a2.TLSRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       wellknown.TLSRouteKind,
			APIVersion: gwv1a2.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

func grpcRoute(name, ns string) *gwv1.GRPCRoute {
	return &gwv1.GRPCRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       wellknown.GRPCRouteKind,
			APIVersion: gwv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

func nsptr(s string) *gwv1.Namespace {
	var ns gwv1.Namespace = gwv1.Namespace(s)
	return &ns
}

var SvcGk = schema.GroupKind{
	Group: corev1.GroupName,
	Kind:  "Service",
}

func newQueries(t test.Failer, initObjs ...client.Object) query.GatewayQueries {
	var anys []any
	for _, obj := range initObjs {
		anys = append(anys, obj)
	}
	mock := krttest.NewMock(t, anys)
	services := krttest.GetMockCollection[*corev1.Service](mock)
	refgrants := krtcollections.NewRefGrantIndex(krttest.GetMockCollection[*gwv1beta1.ReferenceGrant](mock))

	policies := krtcollections.NewPolicyIndex(krtinternal.KrtOptions{}, extensionsplug.ContributesPolicies{}, settings.Settings{})
	upstreams := krtcollections.NewBackendIndex(krtinternal.KrtOptions{}, policies, refgrants)
	upstreams.AddBackends(SvcGk, k8sUpstreams(services))

	httproutes := krttest.GetMockCollection[*gwv1.HTTPRoute](mock)
	tcpproutes := krttest.GetMockCollection[*gwv1a2.TCPRoute](mock)
	tlsroutes := krttest.GetMockCollection[*gwv1a2.TLSRoute](mock)
	grpcroutes := krttest.GetMockCollection[*gwv1.GRPCRoute](mock)
	rtidx := krtcollections.NewRoutesIndex(krtinternal.KrtOptions{}, httproutes, grpcroutes, tcpproutes, tlsroutes, policies, upstreams, refgrants, settings.Settings{})
	services.WaitUntilSynced(nil)

	secretsCol := map[schema.GroupKind]krt.Collection[ir.Secret]{
		corev1.SchemeGroupVersion.WithKind("Secret").GroupKind(): krt.NewCollection(krttest.GetMockCollection[*corev1.Secret](mock), func(kctx krt.HandlerContext, i *corev1.Secret) *ir.Secret {
			res := ir.Secret{
				ObjectSource: ir.ObjectSource{
					Group:     "",
					Kind:      "Secret",
					Namespace: i.Namespace,
					Name:      i.Name,
				},
				Obj:  i,
				Data: i.Data,
			}
			return &res
		}),
	}
	secrets := krtcollections.NewSecretIndex(secretsCol, refgrants)
	nsCol := krtcollections.NewNamespaceCollectionFromCol(context.Background(), krttest.GetMockCollection[*corev1.Namespace](mock), krtinternal.KrtOptions{})

	commonCols := &common.CommonCollections{
		Routes: rtidx, Secrets: secrets, Namespaces: nsCol,
	}

	for !rtidx.HasSynced() || !refgrants.HasSynced() || !secrets.HasSynced() || !upstreams.HasSynced() {
		time.Sleep(time.Second / 10)
	}
	return query.NewData(commonCols)
}

func k8sUpstreams(services krt.Collection[*corev1.Service]) krt.Collection[ir.BackendObjectIR] {
	return krt.NewManyCollection(services, func(kctx krt.HandlerContext, svc *corev1.Service) []ir.BackendObjectIR {
		uss := []ir.BackendObjectIR{}

		for _, port := range svc.Spec.Ports {
			uss = append(uss, ir.BackendObjectIR{
				ObjectSource: ir.ObjectSource{
					Kind:      SvcGk.Kind,
					Group:     SvcGk.Group,
					Namespace: svc.Namespace,
					Name:      svc.Name,
				},
				Obj:  svc,
				Port: port.Port,
			})
		}
		return uss
	})
}

func ls() *gwxv1a1.XListenerSet {
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "ls",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
				{
					Name:     "bar",
					Protocol: gwv1.HTTPProtocolType,
				},
			},
			ParentRef: gwxv1a1.ParentGatewayReference{
				Name: "test",
			},
		},
	}
}
