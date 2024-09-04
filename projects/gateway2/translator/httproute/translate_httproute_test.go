package httproute_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
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
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("GatewayHttpRouteTranslator", func() {
	var (
		ctrl *gomock.Controller
		ctx  context.Context
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		ctx = context.Background()
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	// TODO(tim): DRY this test code up.

	When("translating a basic HTTPRoute", func() {
		var (
			route             gwv1.HTTPRoute
			routeInfo         *query.HTTPRouteInfo
			parentRef         *gwv1.ParentReference
			pluginRegistry    registry.PluginRegistry
			baseReporter      reports.Reporter
			parentRefReporter reports.ParentRefReporter
			gwListener        gwv1.Listener
		)

		BeforeEach(func() {
			fakeClient := testutils.BuildIndexedFakeClient(
				[]client.Object{},
				rtoptquery.IterateIndices,
				vhoptquery.IterateIndices,
				lisquery.IterateIndices,
				httplisquery.IterateIndices,
			)
			queries := testutils.BuildGatewayQueriesWithClient(fakeClient)
			resourceClientFactory := &factory.MemoryResourceClientFactory{
				Cache: memory.NewInMemoryResourceCache(),
			}
			routeOptionClient, _ := sologatewayv1.NewRouteOptionClient(ctx, resourceClientFactory)
			vhOptionClient, _ := sologatewayv1.NewVirtualHostOptionClient(ctx, resourceClientFactory)
			statusClient := statusutils.GetStatusClientForNamespace("gloo-system")
			statusReporter := reporter.NewReporter(defaults.KubeGatewayReporter, statusClient, routeOptionClient.BaseClient())
			pluginRegistry = registry.NewPluginRegistry(registry.BuildPlugins(queries, fakeClient, routeOptionClient, vhOptionClient, statusReporter))

			gwListener = gwv1.Listener{} // Initialize appropriately
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

			reportsMap := reports.NewReportMap()
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
		})
	})

	When("an HTTPRoute configures a backendRef and references the DRR extension filter", func() {
		var (
			drr               *v1alpha1.DirectResponseRoute
			route             gwv1.HTTPRoute
			routeInfo         *query.HTTPRouteInfo
			parentRef         *gwv1.ParentReference
			pluginRegistry    registry.PluginRegistry
			baseReporter      reports.Reporter
			parentRefReporter reports.ParentRefReporter
			gwListener        gwv1.Listener
		)
		BeforeEach(func() {
			gwListener = gwv1.Listener{} // Initialize appropriately

			drr = &v1alpha1.DirectResponseRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "bar",
				},
				Spec: v1alpha1.DirectResponseRouteSpec{
					Status: 200,
				},
			}

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
						ParentRefs: []gwv1.ParentReference{*parentRef},
					},
					Rules: []gwv1.HTTPRouteRule{{
						Matches: []gwv1.HTTPRouteMatch{{
							Path: &gwv1.HTTPPathMatch{
								Type:  ptr.To(gwv1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
						}},
						BackendRefs: []gwv1.HTTPBackendRef{{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: "httpbin",
									Port: ptr.To(gwv1.PortNumber(8000)),
								},
							},
						}},
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
			routeInfo = &query.HTTPRouteInfo{
				HTTPRoute: route,
			}
			reportsMap := reports.NewReportMap()
			baseReporter := reports.NewReporter(&reportsMap)
			parentRefReporter = baseReporter.Route(&route).ParentRef(parentRef)

			fakeClient := testutils.BuildIndexedFakeClient(
				[]client.Object{drr},
				rtoptquery.IterateIndices,
				vhoptquery.IterateIndices,
				lisquery.IterateIndices,
				httplisquery.IterateIndices,
			)

			queries := testutils.BuildGatewayQueriesWithClient(fakeClient)
			resourceClientFactory := &factory.MemoryResourceClientFactory{
				Cache: memory.NewInMemoryResourceCache(),
			}
			routeOptionClient, _ := sologatewayv1.NewRouteOptionClient(ctx, resourceClientFactory)
			vhOptionClient, _ := sologatewayv1.NewVirtualHostOptionClient(ctx, resourceClientFactory)
			statusClient := statusutils.GetStatusClientForNamespace("gloo-system")
			statusReporter := reporter.NewReporter(defaults.KubeGatewayReporter, statusClient, routeOptionClient.BaseClient())
			pluginRegistry = registry.NewPluginRegistry(registry.BuildPlugins(queries, fakeClient, routeOptionClient, vhOptionClient, statusReporter))
		})

		It("replaces the route due to incompatible filters being configured", func() {
			routes := httproute.TranslateGatewayHTTPRouteRules(ctx, pluginRegistry, gwListener, routeInfo, parentRefReporter, baseReporter)
			Expect(routes).To(HaveLen(1))
			Expect(routes[0].GetAction()).To(BeEquivalentTo(directresponse.ErrorResponseAction()))
		})
	})

	When("an HTTPRoute configures multiple route actions", func() {
		var (
			drr               *v1alpha1.DirectResponseRoute
			route             gwv1.HTTPRoute
			routeInfo         *query.HTTPRouteInfo
			parentRef         *gwv1.ParentReference
			pluginRegistry    registry.PluginRegistry
			baseReporter      reports.Reporter
			parentRefReporter reports.ParentRefReporter
			gwListener        gwv1.Listener
		)
		BeforeEach(func() {
			gwListener = gwv1.Listener{} // Initialize appropriately

			drr = &v1alpha1.DirectResponseRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "bar",
				},
				Spec: v1alpha1.DirectResponseRouteSpec{
					Status: 200,
				},
			}

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
						ParentRefs: []gwv1.ParentReference{*parentRef},
					},
					Rules: []gwv1.HTTPRouteRule{{
						Matches: []gwv1.HTTPRouteMatch{{
							Path: &gwv1.HTTPPathMatch{
								Type:  ptr.To(gwv1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
						}},
						Filters: []gwv1.HTTPRouteFilter{
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
						},
					}},
				},
			}
			routeInfo = &query.HTTPRouteInfo{
				HTTPRoute: route,
			}
			reportsMap := reports.NewReportMap()
			baseReporter := reports.NewReporter(&reportsMap)
			parentRefReporter = baseReporter.Route(&route).ParentRef(parentRef)

			fakeClient := testutils.BuildIndexedFakeClient(
				[]client.Object{drr},
				rtoptquery.IterateIndices,
				vhoptquery.IterateIndices,
				lisquery.IterateIndices,
				httplisquery.IterateIndices,
			)

			queries := testutils.BuildGatewayQueriesWithClient(fakeClient)
			resourceClientFactory := &factory.MemoryResourceClientFactory{
				Cache: memory.NewInMemoryResourceCache(),
			}
			routeOptionClient, _ := sologatewayv1.NewRouteOptionClient(ctx, resourceClientFactory)
			vhOptionClient, _ := sologatewayv1.NewVirtualHostOptionClient(ctx, resourceClientFactory)
			statusClient := statusutils.GetStatusClientForNamespace("gloo-system")
			statusReporter := reporter.NewReporter(defaults.KubeGatewayReporter, statusClient, routeOptionClient.BaseClient())
			pluginRegistry = registry.NewPluginRegistry(registry.BuildPlugins(queries, fakeClient, routeOptionClient, vhOptionClient, statusReporter))
		})

		It("should replace the route due to incompatible filters", func() {
			routes := httproute.TranslateGatewayHTTPRouteRules(ctx, pluginRegistry, gwListener, routeInfo, parentRefReporter, baseReporter)
			Expect(routes).To(HaveLen(1))
			Expect(routes[0].GetAction()).To(BeEquivalentTo(directresponse.ErrorResponseAction()))
		})
	})
})
