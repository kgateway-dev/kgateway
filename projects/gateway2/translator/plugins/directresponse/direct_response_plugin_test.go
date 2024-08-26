package directresponse_test

import (
	"context"
	"net/http"

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

var _ = Describe("DirectResponseRoute", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		deps   []client.Object
		c      client.Client
		p      plugins.RoutePlugin
	)
	JustBeforeEach(func() {
		c = testutils.BuildIndexedFakeClient(deps)
		p = directresponse.NewPlugin(c)
	})
	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})
	AfterEach(func() {
		cancel()
	})

	When("a valid direct response route is present", func() {
		var (
			drr *v1alpha1.DirectResponseRoute
		)
		BeforeEach(func() {
			drr = &v1alpha1.DirectResponseRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "httpbin",
				},
				Spec: v1alpha1.DirectResponseRouteSpec{
					Status: ptr.To(uint32(200)),
					Body:   ptr.To(string("hello, world")),
				},
			}
			deps = []client.Object{drr}
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
							Kind:  v1alpha1.DirectResponseRouteKind,
							Name:  gwv1.ObjectName(drr.GetName()),
						},
					}},
				},
				Reporter: parentRefReporter,
			}

			By("verifying the output route has a direct response action")
			err := p.ApplyRoutePlugin(ctx, routeCtx, route)
			Expect(err).NotTo(HaveOccurred())
			Expect(route).ToNot(BeNil())
			Expect(route.GetDirectResponseAction()).To(BeEquivalentTo(&v1.DirectResponseAction{
				Status: 200,
				Body:   "hello, world",
			}))

			By("verifying the HTTPRoute status is set correctly")
			status := reportsMap.BuildRouteStatus(ctx, *rt, "")
			Expect(status).NotTo(BeNil())
			Expect(status.Parents).To(HaveLen(1))
			resolvedRefs := meta.FindStatusCondition(status.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
			Expect(resolvedRefs).NotTo(BeNil())
			Expect(resolvedRefs.Reason).To(BeEquivalentTo(gwv1.RouteReasonResolvedRefs))
			Expect(resolvedRefs.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	When("an HTTPRoute references a non-existent DRR resource", func() {
		var (
			drr *v1alpha1.DirectResponseRoute
		)
		BeforeEach(func() {
			drr = &v1alpha1.DirectResponseRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "httpbin",
				},
				Spec: v1alpha1.DirectResponseRouteSpec{
					Status: ptr.To(uint32(200)),
					Body:   ptr.To(string("hello, world")),
				},
			}
			deps = []client.Object{drr}
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
							Kind:  v1alpha1.DirectResponseRouteKind,
							Name:  "non-existent",
						},
					}},
				},
				Reporter: parentRefReporter,
			}

			By("verifying the output route has no direct response action")
			err := p.ApplyRoutePlugin(ctx, routeCtx, route)
			Expect(err).To(HaveOccurred())
			Expect(route.GetDirectResponseAction()).To(BeEquivalentTo(&v1.DirectResponseAction{
				Status: http.StatusInternalServerError,
			}))

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

	// TODO(tim): determine whether this is the right approach to validation.
	// I previously had test cases for duplicate DRR resource references,
	// one valid and one invalid, and both valid and unique, but I wasn't
	// able to find a real use cases where you'd want to reference multiple
	// DRR resources in a single route rule. The only issue with the new
	// approach is that it's potentially fail open where a user configured a
	// valid HTTPRoute rule, then edits it to include multiple DRR resources,
	// and now the route is broken. With that in mind, I replaced the route with
	// a 500 response, so we're not breaking the route, but we're also not
	// applying the desired state.
	When("an HTTPRoute references multiple DRR resources", func() {
		var (
			drr1, drr2 *v1alpha1.DirectResponseRoute
		)
		BeforeEach(func() {
			drr1 = &v1alpha1.DirectResponseRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "drr1",
					Namespace: "httpbin",
				},
				Spec: v1alpha1.DirectResponseRouteSpec{
					Status: ptr.To(uint32(200)),
					Body:   ptr.To(string("hello from DRR 1")),
				},
			}
			drr2 = &v1alpha1.DirectResponseRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "drr2",
					Namespace: "httpbin",
				},
				Spec: v1alpha1.DirectResponseRouteSpec{
					Status: ptr.To(uint32(404)),
					Body:   ptr.To(string("hello from DRR 2")),
				},
			}
			deps = []client.Object{drr1, drr2}
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
								Kind:  v1alpha1.DirectResponseRouteKind,
								Name:  gwv1.ObjectName(drr1.GetName()),
							},
						},
						{
							Type: gwv1.HTTPRouteFilterExtensionRef,
							ExtensionRef: &gwv1.LocalObjectReference{
								Group: v1alpha1.Group,
								Kind:  v1alpha1.DirectResponseRouteKind,
								Name:  gwv1.ObjectName(drr2.GetName()),
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
			Expect(route.GetDirectResponseAction()).To(BeEquivalentTo(&v1.DirectResponseAction{
				Status: http.StatusInternalServerError,
			}))

			By("verifying the HTTPRoute status is set correctly")
			status := reportsMap.BuildRouteStatus(ctx, *rt, "")
			Expect(status).NotTo(BeNil())
			Expect(status.Parents).To(HaveLen(1))
			resolvedRefs := meta.FindStatusCondition(status.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
			Expect(resolvedRefs).NotTo(BeNil())
			Expect(resolvedRefs.Reason).To(BeEquivalentTo(gwv1.RouteReasonBackendNotFound))
			Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	When("an HTTPRoute references a DRR resource in the backendRef filters", func() {
		var (
			drr *v1alpha1.DirectResponseRoute
		)
		BeforeEach(func() {
			drr = &v1alpha1.DirectResponseRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "httpbin",
				},
				Spec: v1alpha1.DirectResponseRouteSpec{
					Status: ptr.To(uint32(200)),
					Body:   ptr.To(string("hello, world")),
				},
			}
			deps = []client.Object{drr}
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
						Filters: []gwv1.HTTPRouteFilter{{
							Type: gwv1.HTTPRouteFilterExtensionRef,
							ExtensionRef: &gwv1.LocalObjectReference{
								Group: v1alpha1.Group,
								Kind:  v1alpha1.DirectResponseRouteKind,
								Name:  gwv1.ObjectName(drr.GetName()),
							},
						}},
					}},
				},
			}

			By("verifying the backendRef filter was ignored")
			err := p.ApplyRoutePlugin(ctx, routeCtx, route)
			Expect(err).NotTo(HaveOccurred())
			Expect(route).ToNot(BeNil())
			Expect(route.GetDirectResponseAction()).To(BeNil())
		})
	})

	When("an HTTPRoute references a DRR resource in a delegated route", func() {
		// Context: parent route references a DRR resource
		// Context: child route references a DRR resource
		// Context: parent and child references the same DRR resource.
	})
})
