package translator_test

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	errors "github.com/rotisserie/eris"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	"github.com/solo-io/gloo/pkg/utils/statusutils"
	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	. "github.com/solo-io/gloo/projects/gateway2/translator"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	rtoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/routeoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

type TestCase struct {
	Name             string
	InputFiles       []string
	ResultsByGateway map[types.NamespacedName]ExpectedTestResult
}

type ActualTestResult struct {
	Proxy *v1.Proxy
	// Reports     map[types.NamespacedName]*reports.GatewayReport
	//TODO(Law): figure out how RouteReports fit in
}

type ExpectedTestResult struct {
	Proxy string
	// Reports     map[types.NamespacedName]*reports.GatewayReport
}

func (r ExpectedTestResult) Equals(actual ActualTestResult) (bool, error) {
	proxy, err := testutils.ReadProxyFromFile(r.Proxy)
	if err != nil {
		return false, err
	}
	return proxy.Equal(actual.Proxy), nil
}

// map of gwv1.GW namespace/name to translation result
func (tc TestCase) Run(ctx context.Context) (map[types.NamespacedName]bool, error) {
	// load inputs

	var (
		gateways     []*gwv1.Gateway
		dependencies []client.Object
		routeOptions []*solokubev1.RouteOption
	)
	for _, file := range tc.InputFiles {
		objs, err := testutils.LoadFromFiles(ctx, file)
		if err != nil {
			return nil, err
		}
		for _, obj := range objs {
			switch obj := obj.(type) {
			case *gwv1.Gateway:
				gateways = append(gateways, obj)
			case *solokubev1.RouteOption:
				routeOptions = append(routeOptions, obj)
				dependencies = append(dependencies, obj)
			default:
				dependencies = append(dependencies, obj)
			}
		}
	}

	fakeClient := testutils.BuildIndexedFakeClient(dependencies, gwquery.IterateIndices, rtoptquery.IterateIndices)
	queries := testutils.BuildGatewayQueriesWithClient(fakeClient)

	resourceClientFactory := &factory.MemoryResourceClientFactory{
		Cache: memory.NewInMemoryResourceCache(),
	}

	routeOptionClient, _ := sologatewayv1.NewRouteOptionClient(ctx, resourceClientFactory)
	statusClient := statusutils.GetStatusClientForNamespace("gloo-system")
	statusReporter := reporter.NewReporter("gloo-kube-gateway", statusClient, routeOptionClient.BaseClient())
	for _, rtOpt := range routeOptions {
		routeOptionClient.Write(&rtOpt.Spec, clients.WriteOpts{Ctx: ctx})
	}

	pluginRegistry := registry.NewPluginRegistry(registry.BuildPlugins(queries, fakeClient, routeOptionClient, statusReporter))

	results := make(map[types.NamespacedName]bool)
	for _, gw := range gateways {
		gwNN := types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		}
		reportsMap := reports.NewReportMap()
		reporter := reports.NewReporter(&reportsMap)

		// translate gateway
		proxy := NewTranslator(queries, pluginRegistry).TranslateProxy(
			ctx,
			gw,
			defaults.GlooSystem,
			reporter,
		)

		act, _ := testutils.MarshalYaml(proxy)
		fmt.Fprintf(ginkgo.GinkgoWriter, "actual result:\n %s \n", act)

		actReport, _ := testutils.MarshalAnyYaml(reportsMap)
		fmt.Fprintf(ginkgo.GinkgoWriter, "actual reports:\n %s \n", actReport)

		actual := ActualTestResult{
			Proxy: proxy,
			// Reports:     reportsMap.Gateways,
		}

		expected, ok := tc.ResultsByGateway[gwNN]
		if !ok {
			return nil, errors.Errorf("no expected result found for gateway %v", gwNN)
		}

		equal, err := expected.Equals(actual)
		if err != nil {
			return nil, err
		}

		results[gwNN] = equal
	}

	return results, nil
}
