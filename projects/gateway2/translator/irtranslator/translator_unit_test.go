package irtranslator_test

// import (
// 	"testing"

// 	"github.com/google/go-cmp/cmp"
// 	"github.com/google/go-cmp/cmp/cmpopts"
// 	"google.golang.org/protobuf/testing/protocmp"
// 	"k8s.io/apimachinery/pkg/types"
// 	"sigs.k8s.io/controller-runtime/pkg/client"
// 	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

// 	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

// 	v1 "github.com/solo-io/gloo/projects/controller/pkg/api/v1"
// 	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
// 	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
// 	. "github.com/solo-io/gloo/projects/gateway2/translator"
// 	httplisquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/httplisteneroptions/query"
// 	lisquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/listeneroptions/query"
// 	rtoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/routeoptions/query"
// 	vhoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/virtualhostoptions/query"
// 	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
// )

// func CompareProxy2(expectedFile string, actualProxy *v1.Proxy) (string, error) {
// 	expectedProxy, err := testutils.ReadProxyFromFile(expectedFile)
// 	if err != nil {
// 		return "", err
// 	}
// 	return cmp.Diff(expectedProxy, actualProxy, protocmp.Transform(), cmpopts.EquateNaNs()), nil
// }

// func TestTranslate(t *testing.T) {
// 	var (
// 		gateways     []*gwv1.Gateway
// 		dependencies []client.Object
// 	)

// 	for _, obj := range objs {
// 		switch obj := obj.(type) {
// 		case *gwv1.Gateway:
// 			gateways = append(gateways, obj)
// 		case *solokubev1.RouteOption:
// 			// XXX(HACK): We need to set the metadata on the Spec since
// 			// routeOptionClient.Write() calls Validate() internally that
// 			// expects this to be set.
// 			if obj.Spec.Metadata == nil {
// 				obj.Spec.Metadata = &core.Metadata{
// 					Namespace: obj.Namespace,
// 					Name:      obj.Name,
// 				}
// 			}
// 			dependencies = append(dependencies, obj)
// 		default:
// 			dependencies = append(dependencies, obj)
// 		}
// 	}

// 	// TODO(Law): consolidate this with iterators in gateway2/controller.go
// 	fakeClient := testutils.BuildIndexedFakeClient(
// 		dependencies,
// 		gwquery.IterateIndices,
// 		rtoptquery.IterateIndices,
// 		vhoptquery.IterateIndices,
// 		lisquery.IterateIndices,
// 		httplisquery.IterateIndices,
// 	)
// 	queries := testutils.BuildIRQueriesWithClient(fakeClient)

// 	for _, gw := range gateways {
// 		gwNN := types.NamespacedName{
// 			Namespace: gw.Namespace,
// 			Name:      gw.Name,
// 		}

// 		// translate gateway
// 		queries.GetFlattenedRoutes()
// 	}
// }
