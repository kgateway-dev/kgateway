package listener_test

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query/mocks"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/listener"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	pluginsdkreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

var (
	ctx              context.Context
	gwListener       gwv1.Listener
	gateway          *gwv1.Gateway
	listenerReporter pluginsdkreporter.ListenerReporter
	reporter         reports.Reporter
	ml               *listener.MergedListeners
	ctrl             *gomock.Controller
	queries          *mocks.MockGatewayQueries
)

func lisToIr(l gwv1.Listener) ir.Listener {
	return ir.Listener{
		Listener: l,
		Parent:   &gwv1.Gateway{},
	}
}

func tcpToIr(tcpRoute *gwv1a2.TCPRoute) *ir.TcpRouteIR {
	routeir := &ir.TcpRouteIR{
		ObjectSource: ir.ObjectSource{
			Namespace: tcpRoute.Namespace,
			Name:      tcpRoute.Name,
			Kind:      "TCPRoute",
			Group:     gwv1.GroupVersion.Group,
		},
		SourceObject: tcpRoute,
		ParentRefs:   tcpRoute.Spec.ParentRefs,
	}
	if len(tcpRoute.Spec.Rules) == 0 {
		return routeir
	}
	for _, b := range tcpRoute.Spec.Rules[0].BackendRefs {
		routeir.Backends = append(routeir.Backends, ir.BackendRefIR{
			ClusterName:   string(b.Name),
			BackendObject: &ir.BackendObjectIR{},
			Weight:        uint32(ptr.Deref(b.Weight, 1)),
		})
	}

	return routeir
}

func tlsToIr(tlsRoute *gwv1a2.TLSRoute) *ir.TlsRouteIR {
	routeir := &ir.TlsRouteIR{
		ObjectSource: ir.ObjectSource{
			Namespace: tlsRoute.Namespace,
			Name:      tlsRoute.Name,
			Kind:      "TLSRoute",
			Group:     gwv1.GroupVersion.Group,
		},
		SourceObject: tlsRoute,
		ParentRefs:   tlsRoute.Spec.ParentRefs,
	}
	if len(tlsRoute.Spec.Rules) == 0 {
		return routeir
	}
	for _, b := range tlsRoute.Spec.Rules[0].BackendRefs {
		routeir.Backends = append(routeir.Backends, ir.BackendRefIR{
			ClusterName:   string(b.Name),
			BackendObject: &ir.BackendObjectIR{},
			Weight:        uint32(ptr.Deref(b.Weight, 1)),
		})
	}
	return routeir
}

var _ = Describe("Translator TCPRoute Listener", func() {
	Context("TCP", func() {
		BeforeEach(func() {
			ctx = context.Background()
			ctrl = gomock.NewController(GinkgoT())
			queries = mocks.NewMockGatewayQueries(ctrl)

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
				Listeners:        []*listener.MergedListener{},
				Queries:          queries,
				GatewayNamespace: "default",
			}

		})

		AfterEach(func() {
			ctrl.Finish()
		})

		Describe("translates gateway API resources to Gloo proxy listeners", func() {
			It("should create a TCP listener with multiple backend references", func() {
				By("Creating a TCPRoute with multiple backend references")
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

				By("Creating the RouteInfo")
				routes := []*query.RouteInfo{
					{
						Object: tcpToIr(tcpRoute),
					},
				}

				By("Appending the TCP listener")
				ml.AppendTcpListener(lisToIr(gwListener), routes, listenerReporter)

				By("Validating that the TCP listener is properly created with multiple backend references")
				Expect(ml.Listeners).To(HaveLen(1))
				Expect(ml.Listeners[0].TcpFilterChains).To(HaveLen(1))

				// Translate the listener to get the actual Gloo listener
				translatedListener := ml.Listeners[0].TranslateListener(krt.TestingDummyContext{}, ctx, nil, reporter)
				Expect(translatedListener).NotTo(BeNil())
				Expect(translatedListener.TcpFilterChain).To(HaveLen(1))

				tcpListener := translatedListener.TcpFilterChain[0]
				Expect(tcpListener.BackendRefs).To(HaveLen(2))
			})

			It("should log an error for TCPRoute with missing parent reference", func() {
				By("Creating a TCPRoute with no parent references")
				tcpRoute := tcpRoute("test-tcp-route", "default")
				tcpRoute.Spec = gwv1a2.TCPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{}, // Empty ParentRefs to trigger the error
					},
				}

				By("Creating the RouteInfo")
				routes := []*query.RouteInfo{{Object: tcpToIr(tcpRoute)}}

				By("Appending the TCP listener")
				ml.AppendTcpListener(lisToIr(gwListener), routes, listenerReporter)

				By("Validating that no TCP listeners are created")
				Expect(ml.Listeners).To(BeEmpty(), "Expected no listeners due to missing ParentRefs")
			})

			It("should handle TCPRoute with empty backend references", func() {
				By("Creating a TCPRoute with an empty backend references")
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

				By("Creating the RouteInfo")
				routes := []*query.RouteInfo{
					{
						Object: tcpToIr(tcpRoute),
					},
				}

				By("Appending the TCP listener")
				ml.AppendTcpListener(lisToIr(gwListener), routes, listenerReporter)

				By("Validating that a TCP listener is created with no TCPHosts")
				Expect(ml.Listeners).To(HaveLen(1))

				translatedListener := ml.Listeners[0].TranslateListener(krt.TestingDummyContext{}, ctx, nil, reporter)
				Expect(translatedListener).NotTo(BeNil())
				Expect(translatedListener.TcpFilterChain).To(BeEmpty(), "Expected no TCP listeners due to empty backend references")
			})

			It("should not append a listener for an unsupported protocol", func() {
				By("Creating a Listener with an unsupported protocol")
				badListener := gwv1.Listener{
					Name:     "foo-unsupported",
					Protocol: gwv1.ProtocolType("UNSUPPORTED"),
					Port:     8080,
				}

				By("Appending the TCP listener generates an error")
				err := ml.AppendListener(lisToIr(badListener), nil, listenerReporter)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unsupported protocol"))

				By("Validating that TCP listeners is empty due to the unsupported protocol")
				Expect(ml.Listeners).To(BeEmpty(), "Expected no listeners due to unsupported protocol")
			})

			It("should skip routes with invalid parent references and process valid ones", func() {
				By("Creating a TCPRoute with a backend reference")
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

				By("Creating an invalid TCPRoute with no parent references")
				invalidRoute := tcpRoute("invalid-tcp-route", "default")
				invalidRoute.Spec = gwv1a2.TCPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{}, // No parent reference provided
					},
				}

				By("Creating the RouteInfo with valid and invalid TCPRoutes")
				routes := []*query.RouteInfo{
					{
						Object: tcpToIr(validRoute),
					},
					{
						Object: tcpToIr(invalidRoute),
					},
				}

				By("Appending the TCP listener")
				ml.AppendTcpListener(lisToIr(gwListener), routes, listenerReporter)

				By("Validating that one single destination TCP listener is created")
				Expect(ml.Listeners).To(HaveLen(1)) // One valid listener

				translatedListener := ml.Listeners[0].TranslateListener(krt.TestingDummyContext{}, ctx, nil, reporter)
				Expect(translatedListener).NotTo(BeNil())
				Expect(translatedListener.TcpFilterChain).To(HaveLen(1))

				tcpListener := translatedListener.TcpFilterChain[0]
				Expect(tcpListener).NotTo(BeNil())
				Expect(tcpListener.BackendRefs[0]).NotTo(BeNil())
				Expect(tcpListener.BackendRefs[0].ClusterName).To(Equal("backend-svc1"))
			})

			It("should create a TCP listener with a single weighted backend reference", func() {
				By("Creating a weighted TCPRoute with a single backend reference")
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

				By("Creating the RouteInfo")
				routes := []*query.RouteInfo{
					{
						Object: tcpToIr(tcpRoute),
					},
				}

				By("Appending the TCP listener")
				ml.AppendTcpListener(lisToIr(gwListener), routes, listenerReporter)

				By("Validating that one TCP listener is created with a single destination")
				Expect(ml.Listeners).To(HaveLen(1))

				translatedListener := ml.Listeners[0].TranslateListener(krt.TestingDummyContext{}, ctx, nil, reporter)
				Expect(translatedListener).NotTo(BeNil())
				Expect(translatedListener.TcpFilterChain).To(HaveLen(1))

				tcpListener := translatedListener.TcpFilterChain[0]
				Expect(tcpListener).NotTo(BeNil())

				// Access the destination field properly
				Expect(tcpListener.BackendRefs).To(HaveLen(1))
				singleDestination := tcpListener.BackendRefs[0]
				Expect(singleDestination).NotTo(BeNil(), "Expected a single-destination")
				Expect(tcpListener.BackendRefs[0].ClusterName).To(Equal("backend-svc1"))
			})

			It("should create a TCP listener with multiple weighted backend references", func() {
				By("Creating a TCPRoute with multiple weighted backend references")
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

				By("Creating the RouteInfo")
				routes := []*query.RouteInfo{
					{
						Object: tcpToIr(tcpRoute),
					},
				}

				By("Appending the TCP listener")
				ml.AppendTcpListener(lisToIr(gwListener), routes, listenerReporter)

				By("Validating that one TCP listener is created with multiple weighted destinations")
				Expect(ml.Listeners).To(HaveLen(1))

				translatedListener := ml.Listeners[0].TranslateListener(krt.TestingDummyContext{}, ctx, nil, reporter)
				Expect(translatedListener).NotTo(BeNil())
				Expect(translatedListener.TcpFilterChain).To(HaveLen(1))

				tcpListener := translatedListener.TcpFilterChain[0]
				Expect(tcpListener).NotTo(BeNil())

				// Access the multi-destination field
				multiDestination := tcpListener.BackendRefs
				Expect(multiDestination).NotTo(BeNil(), "Expected a multi-destination for weighted backends")

				// Validate that there are two destinations with the correct weights
				Expect(multiDestination).To(HaveLen(2))

				dest1 := multiDestination[0]
				dest2 := multiDestination[1]

				// Ensure backend names, ports, and weights match expectations
				Expect(dest1.ClusterName).To(Equal("backend-svc1"))
				Expect(dest1.Weight).To(Equal(uint32(60)))

				Expect(dest2.ClusterName).To(Equal("backend-svc2"))
				Expect(dest2.Weight).To(Equal(uint32(40)))
			})
		})

		It("should not create a DestinationSpec when backendRef refers to a service in a different namespace without a permitting ReferenceGrant", func() {
			By("Creating a TCPRoute with a backendRef to a different namespace")
			tcpRoute := tcpRoute("cross-namespace-tcp-route", "default")
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
									Name:      "backend-svc",
									Namespace: ptr.To(gwv1.Namespace("other-namespace")),
									Port:      ptr.To(gwv1.PortNumber(8080)),
								},
							},
						},
					},
				},
			}

			By("Setting up the mock to return an error when ReferenceGrant is missing")
			tcpIr := tcpToIr(tcpRoute)
			// simulate missing reference grant
			tcpIr.Backends[0].BackendObject = nil
			tcpIr.Backends[0].Err = errors.New("missing reference grant")

			By("Creating the RouteInfo")
			routes := []*query.RouteInfo{
				{
					Object: tcpIr,
				},
			}

			By("Appending the TCP listener")
			ml.AppendTcpListener(lisToIr(gwListener), routes, listenerReporter)

			By("Validating that a TCP listener is created with no TCPHosts")
			Expect(ml.Listeners).To(HaveLen(1))

			translatedListener := ml.Listeners[0].TranslateListener(krt.TestingDummyContext{}, ctx, nil, reporter)
			Expect(translatedListener).NotTo(BeNil())
			Expect(translatedListener.TcpFilterChain).To(HaveLen(1))

			tcpListener := translatedListener.TcpFilterChain[0]
			Expect(tcpListener).NotTo(BeNil())
			Expect(tcpListener.BackendRefs).To(HaveLen(1))

			tcpHost := tcpListener.BackendRefs[0]
			Expect(tcpHost.BackendObject).To(BeNil())
		})

		/* i think this is not needed, as refgrants are resolved a this point
		It("should create a TCP listener when backendRef refers to a service in a different namespace with a permitting ReferenceGrant", func() {
			By("Creating a TCPRoute with a backendRef to a different namespace")
			tcpRoute := tcpRoute("cross-namespace-tcp-route", "default")
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
									Name:      "backend-svc",
									Namespace: ptr.To(gwv1.Namespace("other-namespace")),
									Port:      ptr.To(gwv1.PortNumber(8080)),
								},
							},
						},
					},
				},
			}

			By("Setting up the mock to return the service when ReferenceGrant allows it")
			queries.EXPECT().
				GetBackendForRef(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, from query.From, ref *gwv1.BackendObjectReference) (client.Object, error) {
					return &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      string(ref.Name),
							Namespace: string(*ref.Namespace),
						},
					}, nil
				}).
				AnyTimes()

			By("Creating the RouteInfo")
			routes := []*query.RouteInfo{
				{
					Object: tcpToIr(tcpRoute),
				},
			}

			By("Appending the TCP listener")
			ml.AppendTcpListener(lisToIr(gwListener), routes, listenerReporter)

			By("Validating that a TCP listener is created with TCPHosts")
			Expect(ml.Listeners).To(HaveLen(1))

			translatedListener := ml.Listeners[0].TranslateListener(krt.TestingDummyContext{}, ctx, nil, reporter)
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
			Expect(tcpHost.Destination.GetSingle().GetKube().GetRef().Name).To(Equal("backend-svc"))
			Expect(tcpHost.Destination.GetSingle().GetKube().GetRef().Namespace).To(Equal("other-namespace"))
		})
		*/
	})
	Context("TLS", func() {
		BeforeEach(func() {
			ctx = context.Background()
			ctrl = gomock.NewController(GinkgoT())
			queries = mocks.NewMockGatewayQueries(ctrl)

			hostname := gwv1.Hostname("example.com")
			gwListener = gwv1.Listener{
				Name:     "foo-tls",
				Protocol: gwv1.TLSProtocolType,
				Port:     443,
				Hostname: &hostname,
			}

			gateway = &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "test-gateway", Namespace: "default"},
			}

			rm := reports.NewReportMap()
			reporter = reports.NewReporter(&rm)
			gatewayReporter := reporter.Gateway(gateway)
			listenerReporter = gatewayReporter.Listener(&gwListener)
			ml = &listener.MergedListeners{
				Listeners:        []*listener.MergedListener{},
				Queries:          queries,
				GatewayNamespace: "default",
			}
		})

		AfterEach(func() {
			ctrl.Finish()
		})

		Describe("translates gateway API resources to Gloo proxy listeners", func() {
			It("should create a TLS listener with multiple backend references", func() {
				tlsRoute := tlsRoute("test-tls-route", "default")
				tlsRoute.Spec = gwv1a2.TLSRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName("test-gateway"),
								Namespace: ptr.To(gwv1.Namespace("default")),
								Kind:      ptr.To(gwv1.Kind(wellknown.GatewayKind)),
							},
						},
					},
					Hostnames: []gwv1a2.Hostname{"example.com"},
					Rules: []gwv1a2.TLSRouteRule{
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

				By("Creating the RouteInfo")
				routes := []*query.RouteInfo{
					{
						Object: tlsToIr(tlsRoute),
					},
				}

				By("Appending the TLS listener")
				ml.AppendTlsListener(lisToIr(gwListener), routes, listenerReporter)

				By("Validating that the TLS listener is properly created with multiple backend references")
				Expect(ml.Listeners).To(HaveLen(1))
				Expect(ml.Listeners[0].TcpFilterChains).To(HaveLen(1))

				translatedListener := ml.Listeners[0].TranslateListener(krt.TestingDummyContext{}, ctx, nil, reporter)
				Expect(translatedListener).NotTo(BeNil())
				Expect(translatedListener.TcpFilterChain).To(HaveLen(1))

				tlsListener := translatedListener.TcpFilterChain[0]
				Expect(tlsListener.BackendRefs).To(HaveLen(2))
				Expect(tlsListener.FilterChainCommon.Matcher.SniDomains).To(ContainElement("example.com"))
			})

			It("should log an error for TLSRoute with missing parent reference", func() {
				By("Creating a TLSRoute with no parent references")
				tlsRoute := tlsRoute("test-tls-route", "default")
				tlsRoute.Spec = gwv1a2.TLSRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{}, // Empty ParentRefs to trigger the error
					},
				}

				By("Creating the RouteInfo")
				routes := []*query.RouteInfo{{Object: tlsToIr(tlsRoute)}}

				By("Appending the TLS listener")
				ml.AppendTlsListener(lisToIr(gwListener), routes, listenerReporter)

				By("Validating that no TLS listeners are created")
				Expect(ml.Listeners).To(BeEmpty(), "Expected no listeners due to missing ParentRefs")
			})

			It("should skip routes with invalid parent references and process valid ones", func() {
				By("Creating a TLSRoute with a backend reference")
				validRoute := tlsRoute("valid-tls-route", "default")
				validRoute.Spec = gwv1a2.TLSRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName("test-gateway"),
								Namespace: ptr.To(gwv1.Namespace("default")),
								Kind:      ptr.To(gwv1.Kind(wellknown.GatewayKind)),
							},
						},
					},
					Rules: []gwv1a2.TLSRouteRule{
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

				By("Creating an invalid TLSRoute with no parent references")
				invalidRoute := tlsRoute("invalid-tls-route", "default")
				invalidRoute.Spec = gwv1a2.TLSRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{}, // No parent reference provided
					},
				}

				By("Creating the RouteInfo with valid and invalid TCPRoutes")
				routes := []*query.RouteInfo{
					{
						Object: tlsToIr(validRoute),
					},
					{
						Object: tlsToIr(invalidRoute),
					},
				}

				By("Appending the TLS listener")
				ml.AppendTlsListener(lisToIr(gwListener), routes, listenerReporter)

				By("Validating that one single destination TLS listener is created")
				Expect(ml.Listeners).To(HaveLen(1)) // One valid listener

				translatedListener := ml.Listeners[0].TranslateListener(krt.TestingDummyContext{}, ctx, nil, reporter)
				Expect(translatedListener).NotTo(BeNil())
				Expect(translatedListener.TcpFilterChain).To(HaveLen(1))

				tlsListener := translatedListener.TcpFilterChain[0]
				Expect(tlsListener).NotTo(BeNil())
				Expect(tlsListener.BackendRefs[0]).NotTo(BeNil())
				Expect(tlsListener.BackendRefs[0].ClusterName).To(Equal("backend-svc1"))
				Expect(tlsListener.FilterChainCommon.Matcher.SniDomains).To(ContainElement("example.com"))
			})

			It("should create a TLS listener with a single weighted backend reference", func() {
				By("Creating a weighted TLSRoute with a single backend reference")
				tlsRoute := tlsRoute("test-tls-route", "default")
				tlsRoute.Spec = gwv1a2.TLSRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName("test-gateway"),
								Namespace: ptr.To(gwv1.Namespace("default")),
								Kind:      ptr.To(gwv1.Kind(wellknown.GatewayKind)),
							},
						},
					},
					Rules: []gwv1a2.TLSRouteRule{
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

				By("Creating the RouteInfo")
				routes := []*query.RouteInfo{
					{
						Object: tlsToIr(tlsRoute),
					},
				}

				By("Appending the TLS listener")
				ml.AppendTlsListener(lisToIr(gwListener), routes, listenerReporter)

				By("Validating that one TLS listener is created with a single destination")
				Expect(ml.Listeners).To(HaveLen(1))

				translatedListener := ml.Listeners[0].TranslateListener(krt.TestingDummyContext{}, ctx, nil, reporter)
				Expect(translatedListener).NotTo(BeNil())
				Expect(translatedListener.TcpFilterChain).To(HaveLen(1))

				tlsListener := translatedListener.TcpFilterChain[0]
				Expect(tlsListener).NotTo(BeNil())

				// Access the destination field properly
				Expect(tlsListener.BackendRefs).To(HaveLen(1))
				singleDestination := tlsListener.BackendRefs[0]
				Expect(singleDestination).NotTo(BeNil(), "Expected a single-destination")
				Expect(tlsListener.BackendRefs[0].ClusterName).To(Equal("backend-svc1"))
				Expect(tlsListener.FilterChainCommon.Matcher.SniDomains).To(ContainElement("example.com"))
			})
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
