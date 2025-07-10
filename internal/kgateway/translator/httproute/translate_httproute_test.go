package httproute_test

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	infextv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/httproute"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	pluginsdkreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

var _ = Describe("GatewayHttpRouteTranslator", func() {
	var (
		ctrl *gomock.Controller
		ctx  context.Context

		// Shared variables for all tests
		up                *ir.BackendObjectIR
		route             *gwv1.HTTPRoute
		routeir           *ir.HttpRouteIR
		routeInfo         *query.RouteInfo
		parentRef         *gwv1.ParentReference
		baseReporter      reports.Reporter
		parentRefReporter pluginsdkreporter.ParentRefReporter
		reportsMap        reports.ReportMap

		// Service backing for routes
		backingSvc *corev1.Service

		// Inferencepool backing for routes
		backingPool *infextv1a2.InferencePool
	)
	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		ctx = context.Background()

		// Common setup for both happy path and negative test cases
		parentRef = &gwv1.ParentReference{
			Name: "my-gw",
		}
		route = &gwv1.HTTPRoute{
			TypeMeta: metav1.TypeMeta{
				Kind:       wellknown.HTTPRouteKind,
				APIVersion: gwv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo-httproute",
				Namespace: "bar",
			},
			Spec: gwv1.HTTPRouteSpec{
				Hostnames: []gwv1.Hostname{"example.com"},
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						*parentRef,
					},
				},
				// Rules are populated in specific test cases
			},
		}
		routeir = &ir.HttpRouteIR{
			ObjectSource: ir.ObjectSource{
				Namespace: route.Namespace,
				Name:      route.Name,
				Kind:      route.Kind,
				Group:     gwv1.GroupVersion.Group,
			},
			SourceObject: route,
			ParentRefs:   []gwv1.ParentReference{*parentRef},
			Hostnames:    []string{"example.com"},
		}

		// “backingSvc” or “backingPool” are assigned in each test’s BeforeEach if needed.

		reportsMap = reports.NewReportMap()
		baseReporter = reports.NewReporter(&reportsMap)
		parentRefReporter = baseReporter.Route(route).ParentRef(parentRef)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Context("HTTPRoute resource routing with Service backendRef", func() {
		When("referencing a valid backing service", func() {
			BeforeEach(func() {
				// Setup the backing service
				backingSvc = &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "bar",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{
							Name: "http",
							Port: 8080,
						}},
					},
				}
				// Setup the backendObjIR
				up = &ir.BackendObjectIR{
					ObjectSource: ir.ObjectSource{
						Namespace: backingSvc.Namespace,
						Name:      backingSvc.Name,
						Kind:      "Service",
						Group:     "",
					},
					Port: 8080,
					Obj:  backingSvc,
				}
				// Setup the route rules
				route.Spec.Rules = []gwv1.HTTPRouteRule{
					{
						Matches: []gwv1.HTTPRouteMatch{
							{Path: &gwv1.HTTPPathMatch{
								Type:  ptr.To(gwv1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							}},
						},
						BackendRefs: []gwv1.HTTPBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name:      gwv1.ObjectName("foo"),
										Namespace: ptr.To(gwv1.Namespace("bar")),
										Kind:      ptr.To(gwv1.Kind("Service")),
										Port:      ptr.To(gwv1.PortNumber(8080)),
									},
								},
							},
						},
					},
				}
				routeir.Rules = []ir.HttpRouteRuleIR{
					{
						Matches: route.Spec.Rules[0].Matches,
						Backends: []ir.HttpBackendOrDelegate{
							{
								Backend: &ir.BackendRefIR{},
							},
						},
					},
				}
				routeir.Rules[0].Backends[0].Backend.BackendObject = up
				routeir.Rules[0].Backends[0].Backend.ClusterName = up.ClusterName()

				// Add backing service to backend map

				// Build RouteInfo
				routeInfo = &query.RouteInfo{
					Object: routeir,
				}
			})

			It("translates the route correctly", func() {
				routes := httproute.TranslateGatewayHTTPRouteRules(ctx, routeInfo, parentRefReporter, baseReporter)

				Expect(routes).To(HaveLen(1))
				Expect(routes[0].Name).To(Equal("httproute-foo-httproute-bar-0-0"))
				Expect(routes[0].Backends[0].Backend.ClusterName).To(Equal(up.ClusterName()))
				Expect(routes[0].Match.Path.Type).To(BeEquivalentTo(ptr.To(gwv1.PathMatchPathPrefix)))
				Expect(routes[0].Match.Path.Value).To(BeEquivalentTo(ptr.To("/")))

				routeStatus := reportsMap.BuildRouteStatus(ctx, route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				By("verifying the route was accepted")
				accepted := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				Expect(accepted).NotTo(BeNil())
				Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
				Expect(accepted.Reason).To(BeEquivalentTo(gwv1.RouteReasonAccepted))
				By("verifying the route was resolved correctly")
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionTrue))
				Expect(resolvedRefs.Reason).To(BeEquivalentTo(gwv1.RouteReasonResolvedRefs))
			})
		})

		When("referencing a non-existent backing service", func() {
			BeforeEach(func() {
				// Setup the backendObjIR
				up = &ir.BackendObjectIR{
					ObjectSource: ir.ObjectSource{
						Namespace: backingSvc.Namespace,
						Name:      backingSvc.Name,
						Kind:      "Service",
						Group:     "",
					},
					Port: 8080,
					Obj:  backingSvc,
				}
				// Setup the route rules
				route.Spec.Rules = []gwv1.HTTPRouteRule{
					{
						Matches: []gwv1.HTTPRouteMatch{
							{Path: &gwv1.HTTPPathMatch{
								Type:  ptr.To(gwv1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							}},
						},
						BackendRefs: []gwv1.HTTPBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name:      gwv1.ObjectName("foo"),
										Namespace: ptr.To(gwv1.Namespace("bar")),
										Kind:      ptr.To(gwv1.Kind("Service")),
										Port:      ptr.To(gwv1.PortNumber(8080)),
									},
								},
							},
						},
					},
				}
				routeir.Rules = []ir.HttpRouteRuleIR{
					{
						Matches: route.Spec.Rules[0].Matches,
						Backends: []ir.HttpBackendOrDelegate{
							{
								Backend: &ir.BackendRefIR{},
							},
						},
					},
				}
				// simulate a missing service
				routeir.Rules[0].Backends[0].Backend.Err = errors.New("missing upstream")
				routeir.Rules[0].Backends[0].Backend.ClusterName = "blackhole_cluster"
				// Build RouteInfo
				routeInfo = &query.RouteInfo{
					Object: routeir,
				}
			})

			It("falls back to a blackhole cluster", func() {
				routes := httproute.TranslateGatewayHTTPRouteRules(ctx, routeInfo, parentRefReporter, baseReporter)

				Expect(routes).To(HaveLen(1))
				Expect(routes[0].Name).To(Equal("httproute-foo-httproute-bar-0-0"))
				Expect(routes[0].Backends[0].Backend.ClusterName).To(Equal("blackhole_cluster"))
				Expect(routes[0].Match.Path.Type).To(BeEquivalentTo(ptr.To(gwv1.PathMatchPathPrefix)))
				Expect(routes[0].Match.Path.Value).To(BeEquivalentTo(ptr.To("/")))

				routeStatus := reportsMap.BuildRouteStatus(ctx, route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				By("verifying the route was accepted")
				accepted := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				Expect(accepted).NotTo(BeNil())
				Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
				Expect(accepted.Reason).To(BeEquivalentTo(gwv1.RouteConditionAccepted))
				By("verifying the route was not able to resolve the backend")
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
				Expect(resolvedRefs.Reason).To(BeEquivalentTo(gwv1.RouteReasonBackendNotFound))
			})
		})
	})

	Context("HTTPRoute resource routing with InferencePool backendRef", func() {
		BeforeEach(func() {
			backingPool = &infextv1a2.InferencePool{
				TypeMeta: metav1.TypeMeta{
					Kind:       wellknown.InferencePoolKind,
					APIVersion: infextv1a2.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-inferencepool",
					Namespace: "bar",
				},
				Spec: infextv1a2.InferencePoolSpec{
					TargetPortNumber: int32(8000),
				},
			}
		})

		When("referencing a valid backing inferencepool", func() {
			BeforeEach(func() {
				route.Spec.Rules = []gwv1.HTTPRouteRule{
					{
						Matches: []gwv1.HTTPRouteMatch{
							{
								Path: &gwv1.HTTPPathMatch{
									Type:  ptr.To(gwv1.PathMatchPathPrefix),
									Value: ptr.To("/"),
								},
							},
						},
						BackendRefs: []gwv1.HTTPBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name:      gwv1.ObjectName(backingPool.Name),
										Namespace: ptr.To(gwv1.Namespace(backingPool.Namespace)),
										Kind:      ptr.To(gwv1.Kind(wellknown.InferencePoolKind)),
										Group:     ptr.To(gwv1.Group(infextv1a2.GroupVersion.Group)),
									},
								},
							},
						},
					},
				}

				// Set a rule referencing the backing InferencePool
				routeir.Rules = []ir.HttpRouteRuleIR{
					{
						Matches: route.Spec.Rules[0].Matches,
						Backends: []ir.HttpBackendOrDelegate{
							{
								Backend: &ir.BackendRefIR{
									BackendObject: &ir.BackendObjectIR{
										ObjectSource: ir.ObjectSource{
											Namespace: backingPool.Namespace,
											Name:      backingPool.Name,
											Kind:      wellknown.InferencePoolKind,
											Group:     infextv1a2.GroupVersion.Group,
										},
										Port: 8000,
										Obj:  backingPool,
									},
									ClusterName: "inferencepool_cluster",
								},
							},
						},
					},
				}

				routeInfo = &query.RouteInfo{
					Object: routeir,
				}
			})

			It("translates the route correctly", func() {
				routes := httproute.TranslateGatewayHTTPRouteRules(
					ctx, routeInfo, parentRefReporter, baseReporter)

				Expect(routes).To(HaveLen(1))
				Expect(routes[0].Name).To(Equal("httproute-foo-httproute-bar-0-0"))

				Expect(routes[0].Backends).To(HaveLen(1))
				Expect(routes[0].Backends[0].Backend.ClusterName).To(Equal("inferencepool_cluster"))

				routeStatus := reportsMap.BuildRouteStatus(ctx, route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				By("verifying the route was accepted")
				accepted := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				Expect(accepted).NotTo(BeNil())
				Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
				Expect(accepted.Reason).To(BeEquivalentTo(gwv1.RouteReasonAccepted))
				By("verifying the route was resolved correctly")
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionTrue))
				Expect(resolvedRefs.Reason).To(BeEquivalentTo(gwv1.RouteReasonResolvedRefs))
			})
		})

		When("referencing a non-existent backing inferencepool", func() {
			BeforeEach(func() {
				route.Spec.Rules = []gwv1.HTTPRouteRule{
					{
						Matches: []gwv1.HTTPRouteMatch{{
							Path: &gwv1.HTTPPathMatch{
								Type:  ptr.To(gwv1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
						}},
						BackendRefs: []gwv1.HTTPBackendRef{{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      "missing-inferencepool",
									Namespace: ptr.To(gwv1.Namespace(backingPool.Namespace)),
									Kind:      ptr.To(gwv1.Kind(wellknown.InferencePoolKind)),
									Group:     ptr.To(gwv1.Group(infextv1a2.GroupVersion.Group)),
									Port:      ptr.To(gwv1.PortNumber(9002)),
								},
							},
						}},
					},
				}

				routeir.Rules = []ir.HttpRouteRuleIR{
					{
						Matches: route.Spec.Rules[0].Matches,
						Backends: []ir.HttpBackendOrDelegate{
							{
								Backend: &ir.BackendRefIR{
									BackendObject: &ir.BackendObjectIR{
										ObjectSource: ir.ObjectSource{
											Namespace: backingPool.Namespace,
											Name:      "missing-inferencepool",
											Kind:      wellknown.InferencePoolKind,
											Group:     infextv1a2.GroupVersion.Group,
										},
										Port: 9002,
									},
									ClusterName: "blackhole_cluster",
									Err:         errors.New("inferencepool not found"),
								},
							},
						},
					},
				}

				routeInfo = &query.RouteInfo{
					Object: routeir,
				}
			})

			It("falls back to a blackhole cluster", func() {
				routes := httproute.TranslateGatewayHTTPRouteRules(
					ctx, routeInfo, parentRefReporter, baseReporter)

				Expect(routes).To(HaveLen(1))
				Expect(routes[0].Backends[0].Backend.ClusterName).To(Equal("blackhole_cluster"))

				routeStatus := reportsMap.BuildRouteStatus(ctx, route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				By("verifying the route was accepted")
				accepted := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				Expect(accepted).NotTo(BeNil())
				Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
				Expect(accepted.Reason).To(BeEquivalentTo(gwv1.RouteConditionAccepted))
				By("verifying the route was not able to resolve the backend")
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
				Expect(resolvedRefs.Reason).To(BeEquivalentTo(gwv1.RouteReasonBackendNotFound))
			})
		})
	})

	Context("multiple route actions", func() {
		// TODO: Multiple route actoins are implemented in plugins now, so these should go to their unit tests
		// or alternatively, to the ir  translator unit tests.

		//		var (
		//			route             *gwv1.HTTPRoute
		//			routeInfo         *query.RouteInfo
		//			baseReporter      reports.Reporter
		//			parentRefReporter reports.ParentRefReporter
		//			reportsMap        reports.ReportMap
		//		)
		//
		//		// Helper function to create a DirectResponse
		//		createDirectResponse := func(name, namespace string, status uint32) *v1alpha1.DirectResponse {
		//			return &v1alpha1.DirectResponse{
		//				ObjectMeta: metav1.ObjectMeta{
		//					Name:      name,
		//					Namespace: namespace,
		//				},
		//				Spec: v1alpha1.DirectResponseSpec{
		//					StatusCode: status,
		//				},
		//			}
		//		}
		//
		//		// Helper function to create a basic HTTPRoute with a ParentRef
		//		createHTTPRoute := func(backendRefs []gwv1.HTTPBackendRef, filters []gwv1.HTTPRouteFilter) *gwv1.HTTPRoute {
		//			parentRef := &gwv1.ParentReference{Name: "my-gw"}
		//
		//			return &gwv1.HTTPRoute{
		//				ObjectMeta: metav1.ObjectMeta{
		//					Name:      "foo-httproute",
		//					Namespace: "bar",
		//				},
		//				Spec: gwv1.HTTPRouteSpec{
		//					Hostnames: []gwv1.Hostname{"example.com"},
		//					CommonRouteSpec: gwv1.CommonRouteSpec{
		//						ParentRefs: []gwv1.ParentReference{*parentRef},
		//					},
		//					Rules: []gwv1.HTTPRouteRule{{
		//						Matches: []gwv1.HTTPRouteMatch{{
		//							Path: &gwv1.HTTPPathMatch{
		//								Type:  ptr.To(gwv1.PathMatchPathPrefix),
		//								Value: ptr.To("/"),
		//							},
		//						}},
		//						BackendRefs: backendRefs,
		//						Filters:     filters,
		//					}},
		//				},
		//			}
		//		}
		//
		//		// Common BeforeEach block for initializing reports and parentRef
		//		BeforeEach(func() {
		//			reportsMap = reports.NewReportMap()
		//			baseReporter = reports.NewReporter(&reportsMap)
		//		})

		/* TODO: this needs to move to the direct response plugin unit tests
		When("an HTTPRoute configures the backendRef and direct response actions", func() {
			var (
				backingSvc *corev1.Service
			)
			BeforeEach(func() {
				backingSvc = &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "bar",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{
							Name: "http",
							Port: 8080,
						}},
					},
				}

				dr := createDirectResponse("test", "bar", 200)
				deps = append(deps, dr)

				backendRefs := []gwv1.HTTPBackendRef{{
					BackendRef: gwv1.BackendRef{
						BackendObjectReference: gwv1.BackendObjectReference{
							Name:      gwv1.ObjectName(backingSvc.GetName()),
							Namespace: ptr.To(gwv1.Namespace(backingSvc.GetNamespace())),
							Kind:      ptr.To(gwv1.Kind("Service")),
							Port:      ptr.To(gwv1.PortNumber(backingSvc.Spec.Ports[0].Port)),
						},
					},
				}}

				filters := []gwv1.HTTPRouteFilter{{
					Type: gwv1.HTTPRouteFilterExtensionRef,
					ExtensionRef: &gwv1.LocalObjectReference{
						Group: v1alpha1.GroupName,
						Kind:  v1alpha1.DirectResponseKind,
						Name:  gwv1.ObjectName(dr.GetName()),
					},
				}}

				route = createHTTPRoute(backendRefs, filters)
				backends := query.NewBackendMap[client.Object]()
				backends.Add(route.Spec.Rules[0].BackendRefs[0].BackendObjectReference, backingSvc)
				routeInfo = &query.RouteInfo{
					Object:   route,
					Backends: backends,
				}
				parentRefReporter = baseReporter.Route(route).ParentRef(&gwv1.ParentReference{Name: "my-gw"})
			})

			It("should replace the route due to incompatible filters being configured", func() {
				routes := httproute.TranslateGatewayHTTPRouteRules(ctx, pluginRegistry, gwListener, routeInfo, parentRefReporter, baseReporter)
				Expect(routes).To(HaveLen(1))
				Expect(routes[0].GetAction()).To(BeEquivalentTo(directresponse.ErrorResponseAction()))

				routeStatus := reportsMap.BuildRouteStatus(ctx, route, "")
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				By("verifying the route was not accepted")
				accepted := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				Expect(accepted).NotTo(BeNil())
				Expect(accepted.Status).To(Equal(metav1.ConditionFalse))
				Expect(accepted.Reason).To(BeEquivalentTo(gwv1.RouteReasonIncompatibleFilters))
				By("verifying the route was resolved correctly")
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionTrue))
				Expect(resolvedRefs.Reason).To(BeEquivalentTo(gwv1.RouteReasonResolvedRefs))
			})
		})
		When("an HTTPRoute configures the redirect and direct response actions", func() {
			BeforeEach(func() {
				dr := createDirectResponse("test", "bar", 200)
				deps = append(deps, dr)

				filters := []gwv1.HTTPRouteFilter{
					{
						Type: gwv1.HTTPRouteFilterRequestRedirect,
						RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
							Hostname:   ptr.To(gwv1.PreciseHostname("foo")),
							StatusCode: ptr.To(301),
						},
					},
					{
						Type: gwv1.HTTPRouteFilterExtensionRef,
						ExtensionRef: &gwv1.LocalObjectReference{
							Group: v1alpha1.GroupName,
							Kind:  v1alpha1.DirectResponseKind,
							Name:  gwv1.ObjectName(dr.GetName()),
						},
					},
				}

				route = createHTTPRoute(nil, filters)
				routeInfo = &query.RouteInfo{Object: route}
				parentRefReporter = baseReporter.Route(route).ParentRef(&gwv1.ParentReference{Name: "my-gw"})
			})

			It("should replace the route due to incompatible filters being configured", func() {
				routes := httproute.TranslateGatewayHTTPRouteRules(ctx, pluginRegistry, gwListener, routeInfo, parentRefReporter, baseReporter)
				Expect(routes).To(HaveLen(1))
				Expect(routes[0].GetAction()).To(BeEquivalentTo(directresponse.ErrorResponseAction()))

				routeStatus := reportsMap.BuildRouteStatus(ctx, route, "")
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				By("verifying the route was not accepted due to incompatible filters")
				accepted := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				Expect(accepted).NotTo(BeNil())
				Expect(accepted.Status).To(Equal(metav1.ConditionFalse))
				Expect(accepted.Reason).To(BeEquivalentTo(gwv1.RouteReasonIncompatibleFilters))
				By("verifying the route was resolved correctly")
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionTrue))
				Expect(resolvedRefs.Reason).To(BeEquivalentTo(gwv1.RouteReasonResolvedRefs))
			})
		})
		*/

		/* TODO: this needs to move to the builtin plugin unit tests
		When("an HTTPRoute configures the redirect and backendRef actions", func() {
			var (
				backingSvc *corev1.Service
			)
			BeforeEach(func() {
				backingSvc = &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "bar",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{
							Name: "http",
							Port: 8080,
						}},
					},
				}

				backendRefs := []gwv1.HTTPBackendRef{{
					BackendRef: gwv1.BackendRef{
						BackendObjectReference: gwv1.BackendObjectReference{
							Name:      gwv1.ObjectName(backingSvc.GetName()),
							Namespace: ptr.To(gwv1.Namespace(backingSvc.GetNamespace())),
							Kind:      ptr.To(gwv1.Kind("Service")),
							Port:      ptr.To(gwv1.PortNumber(backingSvc.Spec.Ports[0].Port)),
						},
					},
				}}

				filters := []gwv1.HTTPRouteFilter{
					{
						Type: gwv1.HTTPRouteFilterRequestRedirect,
						RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
							Hostname:   ptr.To(gwv1.PreciseHostname("foo")),
							StatusCode: ptr.To(301),
						},
					},
				}

				route = createHTTPRoute(backendRefs, filters)
				backends := query.NewBackendMap[client.Object]()
				backends.Add(route.Spec.Rules[0].BackendRefs[0].BackendObjectReference, backingSvc)
				routeInfo = &query.RouteInfo{
					Object:   route,
					Backends: backends,
				}
				parentRefReporter = baseReporter.Route(route).ParentRef(&gwv1.ParentReference{Name: "my-gw"})
			})

			// Note(tim): the current behavior is that the redirect plugin will return an error
			// but the route won't be replaced with a 500 response. Similarly, the HTTPRoute will
			// reflect an ACCEPTED status as the HTTP route translator logs the error but doesn't
			// handle it.
			It("should ignore the redirect filter and translate the backendRef successfully", func() {
				routes := httproute.TranslateGatewayHTTPRouteRules(ctx, pluginRegistry, gwListener, routeInfo, parentRefReporter, baseReporter)
				Expect(routes).To(HaveLen(1))
				Expect(routes[0].GetAction()).To(BeEquivalentTo(&v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &v1.Destination{
								DestinationType: &v1.Destination_Kube{
									Kube: &v1.KubernetesServiceDestination{
										Ref: &core.ResourceRef{
											Name:      backingSvc.GetName(),
											Namespace: backingSvc.GetNamespace(),
										},
										Port: uint32(backingSvc.Spec.Ports[0].Port),
									},
								},
							},
						},
					},
				}))

				routeStatus := reportsMap.BuildRouteStatus(ctx, route, "")
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				By("verifying the route was accepted")
				accepted := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				Expect(accepted).NotTo(BeNil())
				Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
				Expect(accepted.Reason).To(BeEquivalentTo(gwv1.RouteReasonAccepted))
				By("verifying the route was resolved correctly")
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionTrue))
				Expect(resolvedRefs.Reason).To(BeEquivalentTo(gwv1.RouteReasonResolvedRefs))
			})
		})
		*/
	})
})
