package directresponse_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/directresponse"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("DirectResponse", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		deps   []client.Object
		c      client.Client
		p      plugins.RoutePlugin
	)
	JustBeforeEach(func() {
		c = testutils.BuildIndexedFakeClient(deps)
		queries := testutils.BuildGatewayQueriesWithClient(c)
		p = directresponse.NewPlugin(queries)
	})
	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})
	AfterEach(func() {
		cancel()
	})

	When("a valid direct response route is present", func() {
		var (
			dr *v1alpha1.DirectResponse
		)
		BeforeEach(func() {
			dr = &v1alpha1.DirectResponse{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "httpbin",
				},
				Spec: v1alpha1.DirectResponseSpec{
					StatusCode: uint32(200),
					Body:       "hello, world",
				},
			}
			deps = []client.Object{dr}
		})

		It("should apply the direct response route to the route", func() {
			rt := &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "httpbin",
				},
			}
			reportsMap := reports.NewReportMap()
			reporter := reports.NewReporter(&reportsMap)
			parentRefReporter := reporter.Route(rt).ParentRef(&gwv1.ParentReference{
				Name: "parent-gw",
			})
			route := &v1.Route{}

			routeCtx := &plugins.RouteContext{
				Route: rt,
				Rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{{
						Type: gwv1.HTTPRouteFilterExtensionRef,
						ExtensionRef: &gwv1.LocalObjectReference{
							Group: v1alpha1.Group,
							Kind:  v1alpha1.DirectResponseKind,
							Name:  gwv1.ObjectName(dr.GetName()),
						},
					}},
				},
				Reporter: parentRefReporter,
			}

			By("verifying the output route has a direct response action")
			err := p.ApplyRoutePlugin(ctx, routeCtx, route)
			Expect(err).NotTo(HaveOccurred())
			Expect(route).ToNot(BeNil())
			Expect(route.GetAction()).To(BeEquivalentTo(&v1.Route_DirectResponseAction{
				DirectResponseAction: &v1.DirectResponseAction{
					Status: dr.GetStatusCode(),
					Body:   dr.GetBody(),
				},
			}))
		})
	})

	When("an empty spec.body is configured in a DR resource", func() {
		var (
			dr *v1alpha1.DirectResponse
		)
		BeforeEach(func() {
			dr = &v1alpha1.DirectResponse{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-body",
					Namespace: "httpbin",
				},
				Spec: v1alpha1.DirectResponseSpec{
					StatusCode: uint32(404),
					Body:       "",
				},
			}
			deps = []client.Object{dr}
		})

		It("should apply the direct response route to the route", func() {
			rt := &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "httpbin",
				},
			}
			reportsMap := reports.NewReportMap()
			reporter := reports.NewReporter(&reportsMap)
			parentRefReporter := reporter.Route(rt).ParentRef(&gwv1.ParentReference{
				Name: "parent-gw",
			})
			route := &v1.Route{}

			routeCtx := &plugins.RouteContext{
				Route: rt,
				Rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{{
						Type: gwv1.HTTPRouteFilterExtensionRef,
						ExtensionRef: &gwv1.LocalObjectReference{
							Group: v1alpha1.Group,
							Kind:  v1alpha1.DirectResponseKind,
							Name:  gwv1.ObjectName(dr.GetName()),
						},
					}},
				},
				Reporter: parentRefReporter,
			}

			By("verifying the output route has a direct response action")
			err := p.ApplyRoutePlugin(ctx, routeCtx, route)
			Expect(err).NotTo(HaveOccurred())
			Expect(route).ToNot(BeNil())
			Expect(route.GetAction()).To(BeEquivalentTo(&v1.Route_DirectResponseAction{
				DirectResponseAction: &v1.DirectResponseAction{
					Status: dr.GetStatusCode(),
				},
			}))
		})
	})

	When("an HTTPRoute references a non-existent DR resource", func() {
		var (
			dr *v1alpha1.DirectResponse
		)
		BeforeEach(func() {
			dr = &v1alpha1.DirectResponse{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "httpbin",
				},
				Spec: v1alpha1.DirectResponseSpec{
					StatusCode: uint32(200),
					Body:       "hello, world",
				},
			}
			deps = []client.Object{dr}
		})
		It("should produce an error on the HTTPRoute resource", func() {
			rt := &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin",
					Namespace: "httpbin",
				},
			}
			reportsMap := reports.NewReportMap()
			reporter := reports.NewReporter(&reportsMap)
			parentRefReporter := reporter.Route(rt).ParentRef(&gwv1.ParentReference{
				Name: "parent-gw",
			})

			route := &v1.Route{}
			routeCtx := &plugins.RouteContext{
				Route: rt,
				Rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{{
						Type: gwv1.HTTPRouteFilterExtensionRef,
						ExtensionRef: &gwv1.LocalObjectReference{
							Group: v1alpha1.Group,
							Kind:  v1alpha1.DirectResponseKind,
							Name:  "non-existent",
						},
					}},
				},
				Reporter: parentRefReporter,
			}

			By("verifying the output route has no direct response action")
			err := p.ApplyRoutePlugin(ctx, routeCtx, route)
			Expect(err).To(HaveOccurred())
			Expect(route.GetAction()).To(BeEquivalentTo(directresponse.ErrorResponseAction()))

			By("verifying the HTTPRoute status is reflecting an error")
			status := reportsMap.BuildRouteStatus(ctx, *rt, "")
			Expect(status).NotTo(BeNil())
			Expect(status.Parents).To(HaveLen(1))
			resolvedRefs := meta.FindStatusCondition(status.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
			Expect(resolvedRefs).NotTo(BeNil())
			Expect(resolvedRefs.Reason).To(BeEquivalentTo(gwv1.RouteReasonBackendNotFound))
			Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	When("an HTTPRoute references multiple DR resources", func() {
		var (
			dr1, dr2 *v1alpha1.DirectResponse
		)
		BeforeEach(func() {
			dr1 = &v1alpha1.DirectResponse{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dr1",
					Namespace: "httpbin",
				},
				Spec: v1alpha1.DirectResponseSpec{
					StatusCode: uint32(200),
					Body:       "hello from DR 1",
				},
			}
			dr2 = &v1alpha1.DirectResponse{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dr2",
					Namespace: "httpbin",
				},
				Spec: v1alpha1.DirectResponseSpec{
					StatusCode: uint32(404),
					Body:       "hello from DR 2",
				},
			}
			deps = []client.Object{dr1, dr2}
		})

		It("should produce an error on the HTTPRoute resource", func() {
			rt := &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "httpbin",
				},
			}
			reportsMap := reports.NewReportMap()
			reporter := reports.NewReporter(&reportsMap)
			parentRefReporter := reporter.Route(rt).ParentRef(&gwv1.ParentReference{
				Name: "parent-gw",
			})
			route := &v1.Route{}

			routeCtx := &plugins.RouteContext{
				Route: rt,
				Rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterExtensionRef,
							ExtensionRef: &gwv1.LocalObjectReference{
								Group: v1alpha1.Group,
								Kind:  v1alpha1.DirectResponseKind,
								Name:  gwv1.ObjectName(dr1.GetName()),
							},
						},
						{
							Type: gwv1.HTTPRouteFilterExtensionRef,
							ExtensionRef: &gwv1.LocalObjectReference{
								Group: v1alpha1.Group,
								Kind:  v1alpha1.DirectResponseKind,
								Name:  gwv1.ObjectName(dr2.GetName()),
							},
						},
					},
				},
				Reporter: parentRefReporter,
			}

			By("verifying the route was replaced")
			err := p.ApplyRoutePlugin(ctx, routeCtx, route)
			Expect(err).To(HaveOccurred())
			Expect(route).ToNot(BeNil())
			Expect(route.GetAction()).To(BeEquivalentTo(directresponse.ErrorResponseAction()))

			By("verifying the HTTPRoute status is set correctly")
			status := reportsMap.BuildRouteStatus(ctx, *rt, "")
			Expect(status).NotTo(BeNil())
			Expect(status.Parents).To(HaveLen(1))
			resolvedRefs := meta.FindStatusCondition(status.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
			Expect(resolvedRefs).NotTo(BeNil())
			Expect(resolvedRefs.Reason).To(BeEquivalentTo(gwv1.RouteReasonIncompatibleFilters))
			Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	When("an HTTPRoute references a DR resource in the backendRef filters", func() {
		var (
			dr *v1alpha1.DirectResponse
		)
		BeforeEach(func() {
			dr = &v1alpha1.DirectResponse{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "httpbin",
				},
				Spec: v1alpha1.DirectResponseSpec{
					StatusCode: uint32(200),
					Body:       "hello, world",
				},
			}
			deps = []client.Object{dr}
		})
		It("should apply the direct response route to the route", func() {
			rt := &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "httpbin",
				},
			}
			reportsMap := reports.NewReportMap()
			reporter := reports.NewReporter(&reportsMap)
			parentRefReporter := reporter.Route(rt).ParentRef(&gwv1.ParentReference{
				Name: "parent-gw",
			})
			route := &v1.Route{}

			routeCtx := &plugins.RouteContext{
				Route:    rt,
				Reporter: parentRefReporter,
				Rule: &gwv1.HTTPRouteRule{
					BackendRefs: []gwv1.HTTPBackendRef{{
						BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: "httpbin",
								Port: ptr.To(gwv1.PortNumber(8080)),
							},
						},
						Filters: []gwv1.HTTPRouteFilter{{
							Type: gwv1.HTTPRouteFilterExtensionRef,
							ExtensionRef: &gwv1.LocalObjectReference{
								Group: v1alpha1.Group,
								Kind:  v1alpha1.DirectResponseKind,
								Name:  gwv1.ObjectName(dr.GetName()),
							},
						}},
					}},
				},
			}

			By("verifying the backendRef filter was ignored")
			err := p.ApplyRoutePlugin(ctx, routeCtx, route)
			Expect(err).NotTo(HaveOccurred())
			Expect(route).ToNot(BeNil())
			Expect(route.GetAction()).To(BeNil())
		})
	})
})
