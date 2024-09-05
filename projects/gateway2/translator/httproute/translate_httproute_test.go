package httproute_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/gloo/pkg/utils/statusutils"
	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/directresponse"
	httplisquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/httplisteneroptions/query"
	lisquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/listeneroptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	rtoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/routeoptions/query"
	vhoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/virtualhostoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	defaultNamespace = "gloo-system"
)

var _ = Describe("GatewayHttpRouteTranslator", func() {
	var (
		ctrl       *gomock.Controller
		ctx        context.Context
		gwListener gwv1.Listener

		deps                  []client.Object
		c                     client.Client
		queries               query.GatewayQueries
		resourceClientFactory *factory.MemoryResourceClientFactory
		pluginRegistry        registry.PluginRegistry
		routeOptionClient     sologatewayv1.RouteOptionClient
		vhOptionClient        sologatewayv1.VirtualHostOptionClient
		statusClient          resources.StatusClient
		statusReporter        reporter.StatusReporter
	)
	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		ctx = context.Background()
		gwListener = gwv1.Listener{}
		deps = []client.Object{}

		resourceClientFactory = &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}
		routeOptionClient, _ = sologatewayv1.NewRouteOptionClient(ctx, resourceClientFactory)
		vhOptionClient, _ = sologatewayv1.NewVirtualHostOptionClient(ctx, resourceClientFactory)
		statusClient = statusutils.GetStatusClientForNamespace(defaultNamespace)
		statusReporter = reporter.NewReporter(defaults.KubeGatewayReporter, statusClient, routeOptionClient.BaseClient())
	})
	AfterEach(func() {
		ctrl.Finish()
	})

	JustBeforeEach(func() {
		// test cases should modify the `deps` slice to add any additional
		// resources needed for the test. we'll build the rest of the required
		// resources to support the test here.
		c = testutils.BuildIndexedFakeClient(
			deps,
			rtoptquery.IterateIndices,
			vhoptquery.IterateIndices,
			lisquery.IterateIndices,
			httplisquery.IterateIndices,
		)
		queries = testutils.BuildGatewayQueriesWithClient(c)
		pluginRegistry = registry.NewPluginRegistry(registry.BuildPlugins(
			queries,
			c,
			routeOptionClient,
			vhOptionClient,
			statusReporter,
		))
	})

	When("translating a basic HTTPRoute", func() {
		var (
			route             gwv1.HTTPRoute
			routeInfo         *query.HTTPRouteInfo
			parentRef         *gwv1.ParentReference
			baseReporter      reports.Reporter
			parentRefReporter reports.ParentRefReporter
			reportsMap        reports.ReportMap
		)
		BeforeEach(func() {
			parentRef = &gwv1.ParentReference{
				Name: "my-gw",
			}
			route = gwv1.HTTPRoute{
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
					Rules: []gwv1.HTTPRouteRule{
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
											Name: "foo",
											Port: ptr.To(gwv1.PortNumber(8080)),
										},
									},
								},
							},
						},
					},
				},
			}
			routeInfo = &query.HTTPRouteInfo{
				HTTPRoute: route,
			}

			reportsMap = reports.NewReportMap()
			baseReporter := reports.NewReporter(&reportsMap)
			parentRefReporter = baseReporter.Route(&route).ParentRef(parentRef)
		})

		It("translates the route correctly", func() {
			routes := httproute.TranslateGatewayHTTPRouteRules(ctx, pluginRegistry, gwListener, routeInfo, parentRefReporter, baseReporter)

			Expect(routes).To(HaveLen(1))
			Expect(routes[0].Name).To(Equal("foo-httproute-bar-0"))
			Expect(routes[0].Matchers).To(HaveLen(1))
			Expect(routes[0].GetAction()).To(BeEquivalentTo(&v1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Kube{
								Kube: &v1.KubernetesServiceDestination{
									Ref: &core.ResourceRef{
										Name:      "blackhole_cluster",
										Namespace: "blackhole_ns",
									},
									Port: 8080,
								},
							},
						},
					},
				},
			}))
			Expect(routes[0].Matchers[0].PathSpecifier).To(Equal(&matchers.Matcher_Prefix{Prefix: "/"}))

			routeStatus := reportsMap.BuildRouteStatus(ctx, route, "")
			Expect(routeStatus).NotTo(BeNil())
			Expect(routeStatus.Parents).To(HaveLen(1))
			resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
			Expect(resolvedRefs).NotTo(BeNil())
			Expect(resolvedRefs.Status).To(Equal(metav1.ConditionTrue))
			Expect(resolvedRefs.Reason).To(BeEquivalentTo(gwv1.RouteReasonAccepted))
		})
	})

	Context("multiple route actions", func() {
		var (
			route             gwv1.HTTPRoute
			routeInfo         *query.HTTPRouteInfo
			baseReporter      reports.Reporter
			parentRefReporter reports.ParentRefReporter
			reportsMap        reports.ReportMap
		)

		// Helper function to create a DirectResponseRoute
		createDirectResponseRoute := func(name, namespace string, status uint32) *v1alpha1.DirectResponseRoute {
			return &v1alpha1.DirectResponseRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: v1alpha1.DirectResponseRouteSpec{
					Status: status,
				},
			}
		}

		// Helper function to create a basic HTTPRoute with a ParentRef
		createHTTPRoute := func(backendRefs []gwv1.HTTPBackendRef, filters []gwv1.HTTPRouteFilter) gwv1.HTTPRoute {
			parentRef := &gwv1.ParentReference{Name: "my-gw"}

			return gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-httproute",
					Namespace: "bar",
				},
				Spec: gwv1.HTTPRouteSpec{
					Hostnames: []gwv1.Hostname{"example.com"},
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{*parentRef},
					},
					Rules: []gwv1.HTTPRouteRule{{
						Matches: []gwv1.HTTPRouteMatch{{
							Path: &gwv1.HTTPPathMatch{
								Type:  ptr.To(gwv1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
						}},
						BackendRefs: backendRefs,
						Filters:     filters,
					}},
				},
			}
		}

		// Common BeforeEach block for initializing reports and parentRef
		BeforeEach(func() {
			reportsMap = reports.NewReportMap()
			baseReporter = reports.NewReporter(&reportsMap)
		})

		When("an HTTPRoute configures the backendRef and direct response actions", func() {
			BeforeEach(func() {
				drr := createDirectResponseRoute("test", "bar", 200)
				deps = append(deps, drr)

				backendRefs := []gwv1.HTTPBackendRef{{
					BackendRef: gwv1.BackendRef{
						BackendObjectReference: gwv1.BackendObjectReference{
							Name: "httpbin",
							Port: ptr.To(gwv1.PortNumber(8000)),
						},
					},
				}}

				filters := []gwv1.HTTPRouteFilter{{
					Type: gwv1.HTTPRouteFilterExtensionRef,
					ExtensionRef: &gwv1.LocalObjectReference{
						Group: v1alpha1.Group,
						Kind:  v1alpha1.DirectResponseRouteKind,
						Name:  gwv1.ObjectName(drr.GetName()),
					},
				}}

				route = createHTTPRoute(backendRefs, filters)
				routeInfo = &query.HTTPRouteInfo{HTTPRoute: route}
				parentRefReporter = baseReporter.Route(&route).ParentRef(&gwv1.ParentReference{Name: "my-gw"})
			})

			It("should replace the route due to incompatible filters being configured", func() {
				routes := httproute.TranslateGatewayHTTPRouteRules(ctx, pluginRegistry, gwListener, routeInfo, parentRefReporter, baseReporter)
				Expect(routes).To(HaveLen(1))
				Expect(routes[0].GetAction()).To(BeEquivalentTo(directresponse.ErrorResponseAction()))

				routeStatus := reportsMap.BuildRouteStatus(ctx, route, "")
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
				Expect(resolvedRefs.Reason).To(BeEquivalentTo(gwv1.RouteReasonIncompatibleFilters))
			})
		})

		When("an HTTPRoute configures the redirect and direct response actions", func() {
			BeforeEach(func() {
				drr := createDirectResponseRoute("test", "bar", 200)
				deps = append(deps, drr)

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
							Group: v1alpha1.Group,
							Kind:  v1alpha1.DirectResponseRouteKind,
							Name:  gwv1.ObjectName(drr.GetName()),
						},
					},
				}

				route = createHTTPRoute(nil, filters)
				routeInfo = &query.HTTPRouteInfo{HTTPRoute: route}
				parentRefReporter = baseReporter.Route(&route).ParentRef(&gwv1.ParentReference{Name: "my-gw"})
			})

			It("should replace the route due to incompatible filters being configured", func() {
				routes := httproute.TranslateGatewayHTTPRouteRules(ctx, pluginRegistry, gwListener, routeInfo, parentRefReporter, baseReporter)
				Expect(routes).To(HaveLen(1))
				Expect(routes[0].GetAction()).To(BeEquivalentTo(directresponse.ErrorResponseAction()))

				routeStatus := reportsMap.BuildRouteStatus(ctx, route, "")
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
				Expect(resolvedRefs.Reason).To(BeEquivalentTo(gwv1.RouteReasonIncompatibleFilters))
			})
		})
	})
})
