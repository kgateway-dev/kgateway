package translator_test

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/onsi/ginkgo/v2"
	"google.golang.org/protobuf/testing/protocmp"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	"github.com/solo-io/gloo/pkg/utils/statusutils"
	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	. "github.com/solo-io/gloo/projects/gateway2/translator"
	httplisquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/httplisteneroptions/query"
	lisquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/listeneroptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	rtoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/routeoptions/query"
	vhoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/virtualhostoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

type TestCase struct {
	InputFiles []string
}

type ActualTestResult struct {
	Proxy      *v1.Proxy
	ReportsMap reports.ReportMap
}

func CompareProxy(expectedFile string, actualProxy *v1.Proxy) (string, error) {
	expectedProxy, err := testutils.ReadProxyFromFile(expectedFile)
	if err != nil {
		return "", err
	}
	return cmp.Diff(expectedProxy, actualProxy, protocmp.Transform(), cmpopts.EquateNaNs()), nil
}

func (tc TestCase) Run(ctx context.Context) (map[types.NamespacedName]ActualTestResult, error) {
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
				// XXX(HACK): We need to set the metadata on the Spec since
				// routeOptionClient.Write() calls Validate() internally that
				// expects this to be set.
				if obj.Spec.Metadata == nil {
					obj.Spec.Metadata = &core.Metadata{
						Namespace: obj.Namespace,
						Name:      obj.Name,
					}
				}
				routeOptions = append(routeOptions, obj)
				dependencies = append(dependencies, obj)
			default:
				dependencies = append(dependencies, obj)
			}
		}
	}

	// TODO(Law): consolidate this with iterators in gateway2/controller.go
	fakeClient := testutils.BuildIndexedFakeClient(
		dependencies,
		gwquery.IterateIndices,
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
	for _, rtOpt := range routeOptions {
		routeOptionClient.Write(&rtOpt.Spec, clients.WriteOpts{Ctx: ctx})
	}

	pluginRegistry := registry.NewPluginRegistry(registry.BuildPlugins(queries, fakeClient, routeOptionClient, vhOptionClient, statusReporter))

	results := make(map[types.NamespacedName]ActualTestResult)

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

		actual := ActualTestResult{
			Proxy:      proxy,
			ReportsMap: reportsMap,
		}
		results[gwNN] = actual
	}

	return results, nil
}
