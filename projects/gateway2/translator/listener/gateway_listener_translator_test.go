package listener_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/listener"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
)

var (
	gwListener       gwv1.Listener
	gateway          *gwv1.Gateway
	listenerReporter reports.ListenerReporter
	reporter         reports.Reporter
	ml               *listener.MergedListeners
)

var _ = Describe("Translator TCPRoute Listener", func() {
	BeforeEach(func() {
		gwListener = gwv1.Listener{
			Name:     "foo-tcp",
			Protocol: gwv1.TCPProtocolType,
			Port:     8080,
		}

		gateway = &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "test-gateway", Namespace: "default"},
		}

		rm := reports.NewReportMap()
		reporter = reports.NewReporter(&rm)
		gatewayReporter := reporter.Gateway(gateway)
		listenerReporter = gatewayReporter.Listener(&gwListener)
		ml = &listener.MergedListeners{
			Listeners: []*listener.MergedListener{},
		}
	})

	Describe("translates gateway API resources to Gloo proxy listeners", func() {
		It("should create a TCP listener with multiple backend references", func() {
			tcpRoute := tcpRoute("test-tcp-route", "default")
			tcpRoute.Spec = gwv1a2.TCPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name:      gwv1.ObjectName("test-gateway"),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Kind:      ptr.To(gwv1.Kind(wellknown.GatewayKind)),
						},
					},
				},
				Rules: []gwv1a2.TCPRouteRule{
					{
						BackendRefs: []gwv1.BackendRef{
							{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      "backend-svc1",
									Namespace: ptr.To(gwv1.Namespace("default")),
									Port:      ptr.To(gwv1.PortNumber(8081)),
								},
								Weight: ptr.To(int32(50)),
							},
							{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      "backend-svc2",
									Namespace: ptr.To(gwv1.Namespace("default")),
									Port:      ptr.To(gwv1.PortNumber(8082)),
								},
								Weight: ptr.To(int32(50)),
							},
						},
					},
				},
			}

			routes := []*query.RouteInfo{{Object: tcpRoute}}
			ml.AppendTcpListener(gwListener, routes, listenerReporter)

			// Validate that the TCP listener is properly created with multiple backend references
			Expect(ml.Listeners).To(HaveLen(1))
			Expect(ml.Listeners[0].TcpFilterChains).To(HaveLen(1))

			// Translate the listener to get the actual Gloo listener
			translatedListener := ml.Listeners[0].TranslateListener(context.Background(), registry.PluginRegistry{}, nil, reporter)
			Expect(translatedListener).NotTo(BeNil())
			aggregateListener := translatedListener.GetAggregateListener()
			Expect(aggregateListener).NotTo(BeNil())
			Expect(aggregateListener.TcpListeners).To(HaveLen(1))

			matchedTcpListener := aggregateListener.TcpListeners[0]
			tcpListener := matchedTcpListener.TcpListener
			Expect(tcpListener).NotTo(BeNil())
			Expect(tcpListener.TcpHosts).To(HaveLen(1))
			tcpHost := tcpListener.TcpHosts[0]
			multiDest := tcpHost.GetDestination().GetMulti()
			Expect(multiDest).NotTo(BeNil())
			Expect(multiDest.Destinations).To(HaveLen(2))
		})

		It("should log an error for TCPRoute with missing parent reference", func() {
			tcpRoute := tcpRoute("test-tcp-route", "default")
			tcpRoute.Spec = gwv1a2.TCPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{}, // Empty ParentRefs to trigger the error
				},
			}

			routes := []*query.RouteInfo{{Object: tcpRoute}}
			ml.AppendTcpListener(gwListener, routes, listenerReporter)

			// Validate that no listeners are created due to missing ParentRefs
			Expect(ml.Listeners).To(BeEmpty(), "Expected no listeners due to missing ParentRefs")
		})

		It("should handle TCPRoute with empty backend references", func() {
			tcpRoute := tcpRoute("test-empty-backend", "default")
			tcpRoute.Spec = gwv1a2.TCPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name:      gwv1.ObjectName("test-gateway"),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Kind:      ptr.To(gwv1.Kind(wellknown.GatewayKind)),
						},
					},
				},
				Rules: []gwv1a2.TCPRouteRule{
					{BackendRefs: []gwv1.BackendRef{}}, // Empty BackendRefs
				},
			}

			routes := []*query.RouteInfo{{Object: tcpRoute}}
			ml.AppendTcpListener(gwListener, routes, listenerReporter)

			// Validate that the listener is created but no TCP hosts are added
			Expect(ml.Listeners).To(HaveLen(1))

			translatedListener := ml.Listeners[0].TranslateListener(context.Background(), registry.PluginRegistry{}, nil, reporter)
			Expect(translatedListener).NotTo(BeNil())
			aggregateListener := translatedListener.GetAggregateListener()
			Expect(aggregateListener).NotTo(BeNil())
			Expect(aggregateListener.TcpListeners).To(HaveLen(1))

			matchedTcpListener := aggregateListener.TcpListeners[0]
			tcpListener := matchedTcpListener.TcpListener
			Expect(tcpListener).NotTo(BeNil())
			Expect(tcpListener.TcpHosts).To(BeEmpty(), "Expected no TCP hosts due to empty backend references")
		})

		It("should not append a listener for an unsupported protocol", func() {
			badListener := gwv1.Listener{
				Name:     "foo-unsupported",
				Protocol: gwv1.ProtocolType("UNSUPPORTED"),
				Port:     8080,
			}

			gatewayReporter := reporter.Gateway(gateway)
			listenerReporter := gatewayReporter.Listener(&badListener)
			err := ml.AppendListener(badListener, nil, listenerReporter)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported protocol"))

			// Validate that no listeners are created due to the unsupported protocol
			Expect(ml.Listeners).To(BeEmpty(), "Expected no listeners due to unsupported protocol")
		})

		It("should skip routes with invalid parent references and process valid ones", func() {
			// Valid TCP route with a correct parent reference
			validRoute := tcpRoute("valid-tcp-route", "default")
			validRoute.Spec = gwv1a2.TCPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name:      gwv1.ObjectName("test-gateway"),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Kind:      ptr.To(gwv1.Kind(wellknown.GatewayKind)),
						},
					},
				},
				Rules: []gwv1a2.TCPRouteRule{
					{
						BackendRefs: []gwv1.BackendRef{
							{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      "backend-svc1",
									Namespace: ptr.To(gwv1.Namespace("default")),
									Port:      ptr.To(gwv1.PortNumber(8081)),
								},
							},
						},
					},
				},
			}

			// Invalid TCP route with missing parent reference
			invalidRoute := tcpRoute("invalid-tcp-route", "default")
			invalidRoute.Spec = gwv1a2.TCPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{}, // No parent reference provided
				},
			}

			routes := []*query.RouteInfo{
				{Object: validRoute},
				{Object: invalidRoute},
			}
			ml := &listener.MergedListeners{
				Listeners: []*listener.MergedListener{},
			}
			ml.AppendTcpListener(gwListener, routes, listenerReporter)

			// Verify that only the valid route is processed
			Expect(ml.Listeners).To(HaveLen(1)) // One valid listener

			translatedListener := ml.Listeners[0].TranslateListener(context.Background(), registry.PluginRegistry{}, nil, reporter)
			Expect(translatedListener).NotTo(BeNil())
			aggregateListener := translatedListener.GetAggregateListener()
			Expect(aggregateListener).NotTo(BeNil())
			Expect(aggregateListener.TcpListeners).To(HaveLen(1))

			matchedTcpListener := aggregateListener.TcpListeners[0]
			tcpListener := matchedTcpListener.TcpListener
			Expect(tcpListener).NotTo(BeNil())
			Expect(tcpListener.TcpHosts).To(HaveLen(1))
			tcpHost := tcpListener.TcpHosts[0]
			Expect(tcpHost.Destination.GetSingle()).NotTo(BeNil())
			Expect(tcpHost.Destination.GetSingle().GetKube().GetRef().Name).To(Equal("backend-svc1"))
		})

		It("should create a TCP listener with a single weighted backend reference", func() {
			tcpRoute := tcpRoute("test-tcp-route", "default")
			tcpRoute.Spec = gwv1a2.TCPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name:      gwv1.ObjectName("test-gateway"),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Kind:      ptr.To(gwv1.Kind(wellknown.GatewayKind)),
						},
					},
				},
				Rules: []gwv1a2.TCPRouteRule{
					{
						BackendRefs: []gwv1.BackendRef{
							{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      "backend-svc1",
									Namespace: ptr.To(gwv1.Namespace("default")),
									Port:      ptr.To(gwv1.PortNumber(8081)),
								},
								Weight: ptr.To(int32(100)),
							},
						},
					},
				},
			}

			routes := []*query.RouteInfo{{Object: tcpRoute}}
			ml := &listener.MergedListeners{
				Listeners: []*listener.MergedListener{},
			}
			ml.AppendTcpListener(gwListener, routes, listenerReporter)

			// Validate the listener creation
			Expect(ml.Listeners).To(HaveLen(1))

			translatedListener := ml.Listeners[0].TranslateListener(context.Background(), registry.PluginRegistry{}, nil, reporter)
			Expect(translatedListener).NotTo(BeNil())
			aggregateListener := translatedListener.GetAggregateListener()
			Expect(aggregateListener).NotTo(BeNil())
			Expect(aggregateListener.TcpListeners).To(HaveLen(1))

			matchedTcpListener := aggregateListener.TcpListeners[0]
			tcpListener := matchedTcpListener.TcpListener
			Expect(tcpListener).NotTo(BeNil())
			Expect(tcpListener.TcpHosts).To(HaveLen(1))

			// Access the destination field properly
			tcpHost := tcpListener.TcpHosts[0]
			Expect(tcpHost.Destination.GetSingle()).NotTo(BeNil())
			Expect(tcpHost.Destination.GetSingle().GetKube().GetRef().Name).To(Equal("backend-svc1"))
		})

		It("should create a TCP listener with multiple weighted backend references", func() {
			tcpRoute := tcpRoute("test-multi-weighted-tcp-route", "default")
			tcpRoute.Spec = gwv1a2.TCPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name:      gwv1.ObjectName("test-gateway"),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Kind:      ptr.To(gwv1.Kind(wellknown.GatewayKind)),
						},
					},
				},
				Rules: []gwv1a2.TCPRouteRule{
					{
						BackendRefs: []gwv1.BackendRef{
							{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      "backend-svc1",
									Namespace: ptr.To(gwv1.Namespace("default")),
									Port:      ptr.To(gwv1.PortNumber(8081)),
								},
								Weight: ptr.To(int32(60)),
							},
							{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      "backend-svc2",
									Namespace: ptr.To(gwv1.Namespace("default")),
									Port:      ptr.To(gwv1.PortNumber(8082)),
								},
								Weight: ptr.To(int32(40)),
							},
						},
					},
				},
			}

			routes := []*query.RouteInfo{{Object: tcpRoute}}
			ml := &listener.MergedListeners{
				Listeners: []*listener.MergedListener{},
			}
			ml.AppendTcpListener(gwListener, routes, listenerReporter)

			// Validate the listener creation
			Expect(ml.Listeners).To(HaveLen(1))

			translatedListener := ml.Listeners[0].TranslateListener(context.Background(), registry.PluginRegistry{}, nil, reporter)
			Expect(translatedListener).NotTo(BeNil())
			aggregateListener := translatedListener.GetAggregateListener()
			Expect(aggregateListener).NotTo(BeNil())
			Expect(aggregateListener.TcpListeners).To(HaveLen(1))

			matchedTcpListener := aggregateListener.TcpListeners[0]
			tcpListener := matchedTcpListener.TcpListener
			Expect(tcpListener).NotTo(BeNil())
			Expect(tcpListener.TcpHosts).To(HaveLen(1))

			// Access the multi-destination field
			tcpHost := tcpListener.TcpHosts[0]
			multiDestination := tcpHost.Destination.GetMulti()
			Expect(multiDestination).NotTo(BeNil(), "Expected a multi-destination for weighted backends")

			// Validate that there are two destinations with the correct weights
			Expect(multiDestination.Destinations).To(HaveLen(2))

			dest1 := multiDestination.Destinations[0]
			dest2 := multiDestination.Destinations[1]

			// Ensure backend names, ports, and weights match expectations
			Expect(dest1.GetDestination().GetKube().GetRef().Name).To(Equal("backend-svc1"))
			Expect(dest1.GetDestination().GetKube().GetRef().Namespace).To(Equal("default"))
			Expect(dest1.GetDestination().GetKube().GetPort()).To(Equal(uint32(8081)))
			Expect(dest1.Weight.GetValue()).To(Equal(uint32(60)))

			Expect(dest2.GetDestination().GetKube().GetRef().Name).To(Equal("backend-svc2"))
			Expect(dest2.GetDestination().GetKube().GetRef().Namespace).To(Equal("default"))
			Expect(dest2.GetDestination().GetKube().GetPort()).To(Equal(uint32(8082)))
			Expect(dest2.Weight.GetValue()).To(Equal(uint32(40)))
		})
	})
})

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
