package testutils

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned/fake"
	common "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

type AssertReports func(gwNN types.NamespacedName, reportsMap reports.ReportMap)

func TestTranslation(
	t test.Failer,
	ctx context.Context,
	inputFiles []string,
	outputFile string,
	gwNN types.NamespacedName,
	assertReports AssertReports,
	settingsOpts ...SettingsOpts,
) {
	results, err := TestCase{
		InputFiles: inputFiles,
	}.Run(t, ctx, settingsOpts...)
	Expect(err).NotTo(HaveOccurred())
	// TODO allow expecting multiple gateways in the output (map nns -> outputFile?)
	Expect(results).To(HaveLen(1))
	Expect(results).To(HaveKey(gwNN))
	result := results[gwNN]

	//// do a json round trip to normalize the output (i.e. things like omit empty)
	//b, _ := json.Marshal(result.Proxy)
	//var proxy ir.GatewayIR
	//Expect(json.Unmarshal(b, &proxy)).NotTo(HaveOccurred())

	Expect(CompareProxy(outputFile, result.Proxy)).To(BeEmpty())

	if assertReports != nil {
		assertReports(gwNN, result.ReportsMap)
	} else {
		Expect(AreReportsSuccess(gwNN, result.ReportsMap)).NotTo(HaveOccurred())
	}
}

type TestCase struct {
	InputFiles []string
}

type ActualTestResult struct {
	Proxy      *irtranslator.TranslationResult
	ReportsMap reports.ReportMap
}

func CompareProxy(expectedFile string, actualProxy *irtranslator.TranslationResult) (string, error) {
	if os.Getenv("UPDATE_OUTPUTS") == "1" {
		d, err := MarshalAnyYaml(actualProxy)
		if err != nil {
			return "", err
		}
		os.WriteFile(expectedFile, d, 0o644)
	}

	expectedProxy, err := ReadProxyFromFile(expectedFile)
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

	for nns, routeReport := range reportsMap.GRPCRoutes {
		for ref, parentRefReport := range routeReport.Parents {
			for _, c := range parentRefReport.Conditions {
				// most route conditions true is good, except RouteConditionPartiallyInvalid
				if c.Type == string(gwv1.RouteConditionPartiallyInvalid) && c.Status != metav1.ConditionFalse {
					return fmt.Errorf("condition error for grpcroute: %v ref: %v condition: %v", nns, ref, c)
				} else if c.Status != metav1.ConditionTrue {
					return fmt.Errorf("condition error for grpcroute: %v ref: %v condition: %v", nns, ref, c)
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

type SettingsOpts func(*settings.Settings)

func SettingsWithDiscoveryNamespaceSelectors(cfgJson string) SettingsOpts {
	return func(s *settings.Settings) {
		s.DiscoveryNamespaceSelectors = cfgJson
	}
}

func (tc TestCase) Run(t test.Failer, ctx context.Context, settingsOpts ...SettingsOpts) (map[types.NamespacedName]ActualTestResult, error) {
	var (
		anyObjs []runtime.Object
		ourObjs []runtime.Object
	)
	for _, file := range tc.InputFiles {
		objs, err := LoadFromFiles(ctx, file)
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
		gvr.GRPCRoute,
		gvr.Service,
		gvr.Pod,
		gvr.TCPRoute,
		gvr.TLSRoute,
		gvr.ServiceEntry,
		gvr.WorkloadEntry,
		gvr.AuthorizationPolicy,
	} {
		clienttest.MakeCRD(t, cli, crd)
	}
	defer cli.Shutdown()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// ensure classes used in tests exist and point at our controller
	gwClasses := append(wellknown.BuiltinGatewayClasses.UnsortedList(), "example-gateway-class")
	for _, className := range gwClasses {
		cli.GatewayAPI().GatewayV1().GatewayClasses().Create(ctx, &gwv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: string(className),
			},
			Spec: gwv1.GatewayClassSpec{
				ControllerName: wellknown.GatewayControllerName,
			},
		}, metav1.CreateOptions{})
	}

	krtOpts := krtutil.KrtOptions{
		Stop: ctx.Done(),
	}

	st, err := settings.BuildSettings()
	if err != nil {
		return nil, err
	}
	for _, opt := range settingsOpts {
		opt(st)
	}

	commoncol, err := common.NewCommonCollections(
		ctx,
		krtOpts,
		cli,
		ourCli,
		nil,
		wellknown.GatewayControllerName,
		logr.Discard(),
		*st,
	)
	if err != nil {
		return nil, err
	}

	plugins := registry.Plugins(ctx, commoncol)
	// TODO: consider moving the common code to a util that both proxy syncer and this test call
	plugins = append(plugins, krtcollections.NewBuiltinPlugin(ctx))
	extensions := registry.MergePlugins(plugins...)

	gk := schema.GroupKind{
		Group: "",
		Kind:  "test-backend-plugin",
	}
	extensions.ContributesPolicies[gk] = extensionsplug.PolicyPlugin{
		Name: "test-backend-plugin",
	}
	extensions.ContributesBackends[gk] = extensionsplug.BackendPlugin{
		Backends: krt.NewStaticCollection([]ir.BackendObjectIR{
			{
				Port: 80,
				ObjectSource: ir.ObjectSource{
					Kind:      "test-backend-plugin",
					Namespace: "default",
					Name:      "example-svc",
				},
			},
		}),
	}

	commoncol.InitPlugins(ctx, extensions)

	translator := translator.NewCombinedTranslator(ctx, extensions, commoncol)
	translator.Init(ctx)

	cli.RunAndWait(ctx.Done())
	commoncol.GatewayIndex.Gateways.WaitUntilSynced(ctx.Done())

	kubeclient.WaitForCacheSync("routes", ctx.Done(), commoncol.Routes.HasSynced)
	kubeclient.WaitForCacheSync("extensions", ctx.Done(), extensions.HasSynced)
	kubeclient.WaitForCacheSync("commoncol", ctx.Done(), commoncol.HasSynced)
	kubeclient.WaitForCacheSync("translator", ctx.Done(), translator.HasSynced)
	kubeclient.WaitForCacheSync("backends", ctx.Done(), commoncol.BackendIndex.HasSynced)
	kubeclient.WaitForCacheSync("endpoints", ctx.Done(), commoncol.Endpoints.HasSynced)

	time.Sleep(1 * time.Second)

	results := make(map[types.NamespacedName]ActualTestResult)

	for _, gw := range commoncol.GatewayIndex.Gateways.List() {
		gwNN := types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		}

		xdsSnap, reportsMap := translator.TranslateGateway(krt.TestingDummyContext{}, ctx, gw)

		act, _ := MarshalAnyYaml(xdsSnap)
		fmt.Fprintf(ginkgo.GinkgoWriter, "actual result:\n %s \n", act)

		actual := ActualTestResult{
			Proxy:      xdsSnap,
			ReportsMap: reportsMap,
		}
		results[gwNN] = actual
	}

	return results, nil
}
