package gateway_test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/onsi/ginkgo/v2"
	"google.golang.org/protobuf/testing/protocmp"
	"istio.io/istio/pkg/config/schema/gvr"
	kubeclient "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient/clienttest"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/gateway/testutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned/fake"
)

type TestCase struct {
	InputFiles []string
}

type ActualTestResult struct {
	Proxy      *irtranslator.TranslationResult
	ReportsMap reports.ReportMap
}

func CompareProxy(expectedFile string, actualProxy *irtranslator.TranslationResult) (string, error) {
	if os.Getenv("UPDATE_OUTPUTS") == "1" {
		d, err := testutils.MarshalAnyYaml(actualProxy)
		if err != nil {
			return "", err
		}
		os.WriteFile(expectedFile, d, 0644)
	}

	expectedProxy, err := testutils.ReadProxyFromFile(expectedFile)
	if err != nil {
		return "", err
	}
	return cmp.Diff(expectedProxy, actualProxy, protocmp.Transform(), cmpopts.EquateNaNs()), nil
}

func AreReportsSuccess(gwNN types.NamespacedName, reportsMap reports.ReportMap) error {
	for nns, routeReport := range reportsMap.HTTPRoutes {
		for ref, parentRefReport := range routeReport.Parents {
			for _, c := range parentRefReport.Conditions {
				// most route conditions true is good, except RouteConditionPartiallyInvalid
				if c.Type == string(gwv1.RouteConditionPartiallyInvalid) && c.Status != metav1.ConditionFalse {
					return fmt.Errorf("condition error for httproute: %v ref: %v condition: %v", nns, ref, c)
				} else if c.Status != metav1.ConditionTrue {
					return fmt.Errorf("condition error for httproute: %v ref: %v condition: %v", nns, ref, c)
				}
			}
		}
	}
	for nns, routeReport := range reportsMap.TCPRoutes {
		for ref, parentRefReport := range routeReport.Parents {
			for _, c := range parentRefReport.Conditions {
				// most route conditions true is good, except RouteConditionPartiallyInvalid
				if c.Type == string(gwv1.RouteConditionPartiallyInvalid) && c.Status != metav1.ConditionFalse {
					return fmt.Errorf("condition error for tcproute: %v ref: %v condition: %v", nns, ref, c)
				} else if c.Status != metav1.ConditionTrue {
					return fmt.Errorf("condition error for tcproute: %v ref: %v condition: %v", nns, ref, c)
				}
			}
		}
	}

	for nns, routeReport := range reportsMap.TLSRoutes {
		for ref, parentRefReport := range routeReport.Parents {
			for _, c := range parentRefReport.Conditions {
				// most route conditions true is good, except RouteConditionPartiallyInvalid
				if c.Type == string(gwv1.RouteConditionPartiallyInvalid) && c.Status != metav1.ConditionFalse {
					return fmt.Errorf("condition error for tlsroute: %v ref: %v condition: %v", nns, ref, c)
				} else if c.Status != metav1.ConditionTrue {
					return fmt.Errorf("condition error for tlsroute: %v ref: %v condition: %v", nns, ref, c)
				}
			}
		}
	}

	for nns, gwReport := range reportsMap.Gateways {
		for _, c := range gwReport.GetConditions() {
			if c.Status != metav1.ConditionTrue {
				return fmt.Errorf("condition not accepted for gw %v condition: %v", nns, c)
			}
		}
	}

	return nil
}

var (
	_ extensionsplug.GetBackendForRefPlugin = testBackendPlugin{}.GetBackendForRefPlugin
)

type testBackendPlugin struct{}

// GetBackendForRef implements query.BackendRefResolver.
func (tp testBackendPlugin) GetBackendForRefPlugin(kctx krt.HandlerContext, key ir.ObjectSource, port int32) *ir.BackendObjectIR {
	if key.Kind != "test-backend-plugin" {
		return nil
	}
	// doesn't matter as long as its not nil
	return &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "test",
			Kind:      "test-backend-plugin",
			Namespace: "test-backend-plugin-ns",
			Name:      "test-backend-plugin-us",
		},
	}
}

func (tc TestCase) Run(t test.Failer, ctx context.Context) (map[types.NamespacedName]ActualTestResult, error) {
	var (
		anyObjs []runtime.Object
		ourObjs []runtime.Object
	)
	for _, file := range tc.InputFiles {
		objs, err := testutils.LoadFromFiles(ctx, file)
		if err != nil {
			return nil, err
		}
		for i := range objs {
			switch obj := objs[i].(type) {
			case *gwv1.Gateway:
				anyObjs = append(anyObjs, obj)

			default:
				apiversion := reflect.ValueOf(obj).Elem().FieldByName("TypeMeta").FieldByName("APIVersion").String()
				if strings.Contains(apiversion, v1alpha1.GroupName) {
					ourObjs = append(ourObjs, obj)
				} else {
					anyObjs = append(anyObjs, objs[i])
				}
			}
		}
	}

	ourCli := fake.NewClientset(ourObjs...)
	cli := kubeclient.NewFakeClient(anyObjs...)
	for _, crd := range []schema.GroupVersionResource{
		gvr.KubernetesGateway_v1,
		gvr.GatewayClass,
		gvr.HTTPRoute_v1,
		gvr.Service,
		gvr.Pod,
		gvr.TCPRoute,
		gvr.TLSRoute,
	} {
		clienttest.MakeCRD(t, cli, crd)
	}
	defer cli.Shutdown()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	krtOpts := krtutil.KrtOptions{
		Stop: ctx.Done(),
	}

	st, err := settings.BuildSettings()
	if err != nil {
		return nil, err
	}
	commoncol := common.NewCommonCollections(
		krtOpts,
		cli,
		ourCli,
		logr.Discard(),
		*st,
	)

	plugins := registry.Plugins(ctx, commoncol)
	// TODO: consider moving the common code to a util that both proxy syncer and this test call
	plugins = append(plugins, krtcollections.NewBuiltinPlugin(ctx))
	extensions := registry.MergePlugins(plugins...)
	gk := schema.GroupKind{
		Group: "",
		Kind:  "test-backend-plugin"}
	extensions.ContributesPolicies[gk] = extensionsplug.PolicyPlugin{
		Name:             "test-backend-plugin",
		GetBackendForRef: testBackendPlugin{}.GetBackendForRefPlugin,
	}

	isOurGw := func(gw *gwv1.Gateway) bool {
		return true
	}

	gi, ri, ui, ei := krtcollections.InitCollections(ctx, extensions, cli, isOurGw, commoncol.RefGrants, krtOpts)

	translator := translator.NewCombinedTranslator(ctx, extensions, commoncol)
	translator.Init(ctx, ri)

	cli.RunAndWait(ctx.Done())
	gi.Gateways.WaitUntilSynced(ctx.Done())
	kubeclient.WaitForCacheSync("routes", ctx.Done(), ri.HasSynced)
	kubeclient.WaitForCacheSync("extensions", ctx.Done(), extensions.HasSynced)
	kubeclient.WaitForCacheSync("commoncol", ctx.Done(), commoncol.HasSynced)
	kubeclient.WaitForCacheSync("translator", ctx.Done(), translator.HasSynced)
	kubeclient.WaitForCacheSync("backends", ctx.Done(), ui.HasSynced)
	kubeclient.WaitForCacheSync("endpoints", ctx.Done(), ei.HasSynced)

	results := make(map[types.NamespacedName]ActualTestResult)

	for _, gw := range gi.Gateways.List() {
		gwNN := types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		}

		xdsSnap, reportsMap := translator.TranslateGateway(krt.TestingDummyContext{}, ctx, gw)

		act, _ := testutils.MarshalAnyYaml(xdsSnap)
		fmt.Fprintf(ginkgo.GinkgoWriter, "actual result:\n %s \n", act)

		actual := ActualTestResult{
			Proxy:      xdsSnap,
			ReportsMap: reportsMap,
		}
		results[gwNN] = actual
	}

	return results, nil
}
