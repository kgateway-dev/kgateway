package translator_test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/onsi/ginkgo/v2"
	"google.golang.org/protobuf/testing/protocmp"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/gvr"
	kubeclient "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/extensions2/common"
	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/extensions2/registry"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/pkg/client/clientset/versioned/fake"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	. "github.com/solo-io/gloo/projects/gateway2/translator"
	"github.com/solo-io/gloo/projects/gateway2/translator/irtranslator"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient/clienttest"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
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
func (tp testBackendPlugin) GetBackendForRefPlugin(kctx krt.HandlerContext, key ir.ObjectSource, port int32) *ir.Upstream {

	if key.Kind != "test-backend-plugin" {
		return nil
	}
	// doesn't matter as long as its not nil
	return &ir.Upstream{
		ObjectSource: ir.ObjectSource{
			Group:     "test",
			Kind:      "test-backend-plugin",
			Namespace: "test-backend-plugin-ns",
			Name:      "test-backend-plugin-us",
		},
	}
}

func registerTypes(ourCli *fake.Clientset) {
	skubeclient.Register[*gwv1.HTTPRoute](
		gvr.HTTPRoute_v1,
		gvk.HTTPRoute_v1.Kubernetes(),
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1().HTTPRoutes(namespace).List(context.Background(), o)
		},
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1().HTTPRoutes(namespace).Watch(context.Background(), o)
		},
	)
	skubeclient.Register[*gwv1a2.TCPRoute](
		gvr.TCPRoute,
		gvk.TCPRoute.Kubernetes(),
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1alpha2().TCPRoutes(namespace).List(context.Background(), o)
		},
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1alpha2().TCPRoutes(namespace).Watch(context.Background(), o)
		},
	)
}

func (tc TestCase) Run(t test.Failer, ctx context.Context) (map[types.NamespacedName]ActualTestResult, error) {
	var (
		anyObjs  []runtime.Object
		ourObjs  []runtime.Object
		gateways []*gwv1.Gateway
	)
	for _, file := range tc.InputFiles {
		objs, err := testutils.LoadFromFiles(ctx, file)
		if err != nil {
			return nil, err
		}
		for i := range objs {
			switch obj := objs[i].(type) {
			case *gwv1.Gateway:
				// due to a problem with the test pluralizer making the gateway resource be `gatewaies`
				// we don't use gateways in the fake client, creating a static collection instead
				gateways = append(gateways, obj)

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
	registerTypes(ourCli)
	cli := kubeclient.NewFakeClient(anyObjs...)
	for _, crd := range []schema.GroupVersionResource{
		gvr.KubernetesGateway_v1,
		gvr.GatewayClass,
		gvr.HTTPRoute_v1,
		gvr.Service,
		gvr.Pod,
	} {
		clienttest.MakeCRD(t, cli, crd)
	}
	defer cli.Shutdown()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	krtOpts := krtutil.KrtOptions{
		Stop: ctx.Done(),
	}

	refgrantsCol := krt.WrapClient(kclient.New[*gwv1beta1.ReferenceGrant](cli), krtOpts.ToOptions("RefGrants")...)
	refGrants := krtcollections.NewRefGrantIndex(refgrantsCol)

	secretClient := kclient.New[*corev1.Secret](cli)
	k8sSecretsRaw := krt.WrapClient(secretClient, krt.WithStop(ctx.Done()), krt.WithName("Secrets") /* no debug here - we don't want raw secrets printed*/)
	k8sSecrets := krt.NewCollection(k8sSecretsRaw, func(kctx krt.HandlerContext, i *corev1.Secret) *ir.Secret {
		res := ir.Secret{
			ObjectSource: ir.ObjectSource{
				Group:     "",
				Kind:      "Secret",
				Namespace: i.Namespace,
				Name:      i.Name,
			},
			Obj:  i,
			Data: i.Data,
		}
		return &res
	}, krtOpts.ToOptions("secrets")...)
	secrets := map[schema.GroupKind]krt.Collection[ir.Secret]{
		{Group: "", Kind: "Secret"}: k8sSecrets,
	}

	augmentedPods := krtcollections.NewPodsCollection(ctx, cli, krtOpts.Debugger)

	s := &glookubev1.Settings{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "settings",
			Namespace: "gloo-system",
		},
	}
	setting := krt.NewStatic(&s, true).AsCollection()

	settingsSingle := krt.NewSingleton(func(ctx krt.HandlerContext) *glookubev1.Settings {
		s := krt.FetchOne(ctx, setting,
			krt.FilterObjectName(types.NamespacedName{Namespace: "gloo-system", Name: "settings"}))
		if s != nil {
			return *s
		}
		return nil
	}, krt.WithName("GlooSettingsSingleton"))
	secretsIdx := krtcollections.NewSecretIndex(secrets, refGrants)
	commoncol := common.CommonCollections{
		OurClient: ourCli,
		Client:    cli,
		KrtOpts:   krtOpts,
		Secrets:   secretsIdx,
		Pods:      augmentedPods,
		Settings:  settingsSingle,
		RefGrants: refGrants,
	}
	nsCol := krtcollections.NewNamespaceCollection(ctx, cli, krtOpts)
	plugins := registry.Plugins(ctx, &commoncol)
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

	rawGateways := krt.NewStaticCollection(gateways)

	httpRoutes := krt.WrapClient(kclient.New[*gwv1.HTTPRoute](cli), krtOpts.ToOptions("httpRoutes")...)
	tcpRoutes := krt.WrapClient(kclient.New[*gwv1a2.TCPRoute](cli), krtOpts.ToOptions("tcpRoutes")...)
	isOurGw := func(gw *gwv1.Gateway) bool {
		return true
	}
	gi, ri, ui, ei := krtcollections.InitCollectionsWithGateways(ctx, isOurGw, rawGateways, httpRoutes, tcpRoutes, refGrants, extensions, krtOpts)
	cli.RunAndWait(ctx.Done())
	gi.Gateways.Synced().WaitUntilSynced(ctx.Done())
	kubeclient.WaitForCacheSync("routes", ctx.Done(), ri.HasSynced)
	kubeclient.WaitForCacheSync("extensions", ctx.Done(), extensions.HasSynced)
	kubeclient.WaitForCacheSync("upstreams", ctx.Done(), ui.Synced().HasSynced)
	kubeclient.WaitForCacheSync("endpoints", ctx.Done(), ei.Synced().HasSynced)
	kubeclient.WaitForCacheSync("namespaces", ctx.Done(), nsCol.Synced().HasSynced)

	queries := query.NewData(ri, secretsIdx, nsCol)

	results := make(map[types.NamespacedName]ActualTestResult)

	for _, gw := range gi.Gateways.List() {
		gwNN := types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		}
		reportsMap := reports.NewReportMap()
		reporter := reports.NewReporter(&reportsMap)

		// translate gateway
		proxy := NewTranslator(queries).Translate(
			krt.TestingDummyContext{},
			ctx,
			&gw,
			reporter,
		)

		xdsTranslator := &irtranslator.Translator{
			ContributedPolicies: extensions.ContributesPolicies,
		}
		xdsSnap := xdsTranslator.Translate(*proxy, reporter)

		act, _ := testutils.MarshalAnyYaml(xdsSnap)
		fmt.Fprintf(ginkgo.GinkgoWriter, "actual result:\n %s \n", act)

		actual := ActualTestResult{
			Proxy:      &xdsSnap,
			ReportsMap: reportsMap,
		}
		results[gwNN] = actual
	}

	return results, nil
}
