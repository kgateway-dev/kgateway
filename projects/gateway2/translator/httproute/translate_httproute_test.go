package httproute_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/solo-io/gloo/pkg/utils/statusutils"
	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute"
	httplisquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/httplisteneroptions/query"
	lisquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/listeneroptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	rtoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/routeoptions/query"
	vhoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/virtualhostoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("GatewayHttpRouteTranslator", func() {
	var (
		ctrl              *gomock.Controller
		ctx               context.Context
		pluginRegistry    registry.PluginRegistry
		baseReporter      reports.Reporter
		parentRefReporter reports.ParentRefReporter
		gwListener        gwv1.Listener
		route             gwv1.HTTPRoute
		routeInfo         *query.HTTPRouteInfo
		parentRef         *gwv1.ParentReference
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		ctx = context.TODO()

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

	AfterEach(func() {
		ctrl.Finish()
	})

	It("translates gateway HTTPRoute into Route correctly", func() {
		routes := httproute.TranslateGatewayHTTPRouteRules(ctx, pluginRegistry, gwListener, routeInfo, parentRefReporter, baseReporter)

		Expect(routes).To(HaveLen(1))
		Expect(routes[0].Name).To(Equal("foo-httproute-bar-0"))
		Expect(routes[0].Matchers).To(HaveLen(1))
		Expect(routes[0].Matchers[0].PathSpecifier).To(Equal(&matchers.Matcher_Prefix{Prefix: "/"}))
	})
})
