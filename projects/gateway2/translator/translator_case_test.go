package translator_test

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	. "github.com/solo-io/gloo/projects/gateway2/translator"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
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
	Proxy   *v1.Proxy
	Reports map[string]*reports.GatewayReport
}

type ExpectedTestResult struct {
	Proxy   string
	Reports map[string]*reports.GatewayReport
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
		proxy := NewTranslator().TranslateProxy(
			ctx,
			gw,
			queries,
			reporter,
		)

		act, _ := testutils.MarshalYaml(proxy)
		fmt.Fprintf(ginkgo.GinkgoWriter, "actual result:\n %s \n", act)

		actReport, _ := testutils.MarshalAnyYaml(reportsMap)
		fmt.Fprintf(ginkgo.GinkgoWriter, "actual reports:\n %s \n", actReport)

		actual := ActualTestResult{
			Proxy:   proxy,
			Reports: reportsMap,
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
