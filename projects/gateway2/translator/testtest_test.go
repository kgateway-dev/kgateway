package translator_test

import (
	"context"
	"testing"

	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/gvr"
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	kubeclient "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
	"istio.io/istio/pkg/kube/kclient/clienttest"
)

func xTestRun(t *testing.T) {

	ctx := context.Background()
	ctx = ctx
	if true {
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
	}
	cli := kubeclient.NewFakeClient(&gwv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRoute",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: gwv1.HTTPRouteSpec{
			Hostnames: []gwv1.Hostname{"test"},
		},
	})
	for _, crd := range []schema.GroupVersionResource{
		gvr.KubernetesGateway_v1,
		gvr.GatewayClass,
		gvr.HTTPRoute_v1,
		gvr.Service,
		gvr.Pod,
	} {
		clienttest.MakeCRD(t, cli, crd)
	}
	t.Cleanup(cli.Shutdown)
	krtOpts := krtutil.KrtOptions{}

	kubeRawGateways := krt.WrapClient(kclient.New[*gwv1.HTTPRoute](cli), krtOpts.ToOptions("KubeGateways")...)
	//	kubeRawGateways := krt.WrapClient(kclient.NewDelayedInformer[*gwv1.HTTPRoute](cli, gvr.KubernetesGateway_v1, kubetypes.DynamicInformer, kclient.Filter{}), krtOpts.ToOptions("KubeGateways")...)

	//kubeRawGateways := krtutil.SetupCollectionDynamic[gwv1.HTTPRoute](
	//	ctx,
	//	cli,
	//	gvr.KubernetesGateway_v1,
	//	krtOpts.ToOptions("KubeGateways")...,
	//)

	cli.RunAndWait(test.NewStop(t))
	//	kubeRawGateways.Synced().WaitUntilSynced(ctx.Done())

	for _, gw := range kubeRawGateways.List() {
		t.Log(gw.Name)
	}
	if len(kubeRawGateways.List()) == 0 {
		t.Fatalf("no gateways found")
	}
}
