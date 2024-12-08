package ir_test

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/gloo/pkg/schemes"
	v1 "github.com/solo-io/gloo/projects/controller/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/model"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/ir"
	httplisquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/httplisteneroptions/query"
	lisquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/listeneroptions/query"
	rtoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/routeoptions/query"
	vhoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/virtualhostoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
)

func CompareProxy2(expectedFile string, actualProxy *v1.Proxy) (string, error) {
	expectedProxy, err := testutils.ReadProxyFromFile(expectedFile)
	if err != nil {
		return "", err
	}
	return cmp.Diff(expectedProxy, actualProxy, protocmp.Transform(), cmpopts.EquateNaNs()), nil
}

func Run(inputFiles []string) {
	var (
		dependencies []client.Object
		gateways     []*gwv1.Gateway
	)
	for _, file := range inputFiles {
		objs, err := testutils.LoadFromFiles(context.TODO(), file)
		if err != nil {
			// return nil, err
			fmt.Println(err)
		}
		for _, obj := range objs {
			if gw, ok := obj.(*gwv1.Gateway); ok {
				gateways = append(gateways, gw)
			}
			dependencies = append(dependencies, obj)
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

	q := ir.NewData(fakeClient, schemes.TestingScheme())
	results := map[string][]*model.HttpRouteRuleMatchIR{}
	for _, gw := range gateways {
		routes, err := q.GetRoutesForGateway(context.TODO(), gw)
		if err != nil {
			// return nil, err
			fmt.Println(err)
		}
		for _, lis := range gw.Spec.Listeners {
			lisRes := routes.ListenerResults[string(lis.Name)]
			for host, irs := range q.GetFlattenedRoutes(lisRes.Routes) {
				results[host] = append(results[host], irs...)
			}
		}
	}
}

func TestBasicGw(t *testing.T) {
	dir := MustGetThisDir()
	i := []string{filepath.Join(dir, "testutils/inputs/", "basic.yaml")}
	Run(i)
}

// taken from github.com/solo-io/skv2@v0.41.0/codegen/util/project_util.go
func MustGetThisDir() string {
	_, thisFile, _, ok := runtime.Caller(1)
	if !ok {
		panic("Failed to get runtime.Caller")
	}
	return filepath.Dir(thisFile)
}
