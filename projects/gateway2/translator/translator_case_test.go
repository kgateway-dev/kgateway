package translator_test

import (
	"context"
	"fmt"
	"sync"

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

	"github.com/solo-io/gloo/projects/gateway2/extensions2/common"
	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/extensions2/registry"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
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
)

type TestCase struct {
	InputFiles []string
}

type ActualTestResult struct {
	Proxy      *irtranslator.TranslationResult
	ReportsMap reports.ReportMap
}

func CompareProxy(expectedFile string, actualProxy *irtranslator.TranslationResult) (string, error) {
	expectedProxy, err := testutils.ReadProxyFromFile(expectedFile)
	if err != nil {
		return "", err
	}
	return cmp.Diff(expectedProxy, actualProxy, protocmp.Transform(), cmpopts.EquateNaNs()), nil
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

func registerTypes() {
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

var registerOnce sync.Once

func (tc TestCase) Run(t test.Failer, ctx context.Context) (map[types.NamespacedName]ActualTestResult, error) {
	var (
		anyObjs  []runtime.Object
		gateways []*gwv1.Gateway
	)
	registerOnce.Do(registerTypes)
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
				anyObjs = append(anyObjs, objs[i])
			}
		}
	}

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
	commoncol := common.CommonCollections{
		Client:   cli,
		KrtOpts:  krtOpts,
		Secrets:  krtcollections.NewSecretIndex(secrets),
		Pods:     augmentedPods,
		Settings: settingsSingle,
	}

	extensions := registry.AllPlugins(ctx, &commoncol)
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

	gi, ri, ui, ei := krtcollections.InitCollectionsWithGateways(ctx, rawGateways, httpRoutes, tcpRoutes, extensions, cli, krtOpts)
	cli.RunAndWait(ctx.Done())
	gi.Gateways.Synced().WaitUntilSynced(ctx.Done())
	kubeclient.WaitForCacheSync("routes", ctx.Done(), ri.HasSynced)
	kubeclient.WaitForCacheSync("extensions", ctx.Done(), extensions.HasSynced)
	kubeclient.WaitForCacheSync("upstreams", ctx.Done(), ui.Synced().HasSynced)
	kubeclient.WaitForCacheSync("endpoints", ctx.Done(), ei.Synced().HasSynced)

	queries := query.NewData(ri) // testutils.BuildGatewayQueriesWithClient(fakeClient, query.WithBackendRefResolvers(&testBackendPlugin{}))

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
		rm := reports.NewReportMap()
		r := reports.NewReporter(&rm)
		xdsSnap := xdsTranslator.Translate(*proxy, r)

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
