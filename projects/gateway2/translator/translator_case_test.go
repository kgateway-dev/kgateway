package translator_test

import (
	"context"
	"log"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	. "github.com/solo-io/gloo/projects/gateway2/translator"
	"github.com/solo-io/gloo/projects/gateway2/translator/listener"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type TestCase struct {
	Name             string
	InputFiles       []string
	ResultsByGateway map[types.NamespacedName]ExpectedTestResult
}

type ActualTestResult struct {
	ListenerAndRoutes []listener.ListenerAndRoutes
	Reports           map[string]*reports.GatewayReport
}

type ExpectedTestResult struct {
	ExpectedOutput string
	Reports        map[string]*reports.GatewayReport
}

func (r ExpectedTestResult) Equals(actual ActualTestResult) (bool, error) {
	proxy, err := testutils.ReadProxyFromFile(r.ExpectedOutput)
	if err != nil {
		return false, err
	}
	return proxy.Equal(actual.ListenerAndRoutes), nil
}

// map of gwv1.GW namespace/name to translation result
func (tc TestCase) Run(ctx context.Context, logActual bool) (map[types.NamespacedName]bool, error) {
	// load inputs

	var (
		gateways     []*gwv1.Gateway
		dependencies []client.Object
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
			default:
				dependencies = append(dependencies, obj)
			}
		}
	}

	queries := testutils.BuildGatewayQueries(dependencies)

	results := make(map[types.NamespacedName]bool)
	for _, gw := range gateways {

		ref := types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		}
		reporter, reportsMap := testutils.BuildReporter()

		// translate gateway
		listenerRoutes := NewTranslator().TranslateProxy(
			ctx,
			gw,
			queries,
			reporter,
		)

		if logActual {
			actualYam, err := testutils.MarshalYaml(listenerRoutes)
			if err != nil {
				return nil, err
			}
			log.Print("actualYam: \n---\n", string(actualYam), "\n---\n")
		}

		actual := ActualTestResult{
			ListenerAndRoutes: listenerRoutes,
			Reports:           reportsMap,
		}

		expected, ok := tc.ResultsByGateway[ref]
		if !ok {
			return nil, errors.Errorf("no expected result found for gateway %v", ref)
		}

		equal, err := expected.Equals(actual)
		if err != nil {
			return nil, err
		}

		results[ref] = equal
	}

	return results, nil
}
