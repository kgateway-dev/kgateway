package krtcollections

import (
	"context"

	"istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
)

func NewGatewayExtensionsCollection(
	ctx context.Context,
	client kube.Client,
	ourClient versioned.Interface,
	krtOpts krtutil.KrtOptions,
) krt.Collection[ir.GatewayExtension] {
	kubeclient.Register[*v1alpha1.GatewayExtension](
		wellknown.GatewayExtensionGVR,
		wellknown.GatewayExtensionGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return ourClient.GatewayV1alpha1().GatewayExtensions(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return ourClient.GatewayV1alpha1().GatewayExtensions(namespace).Watch(context.Background(), o)
		},
	)

	rawGwExts := krt.WrapClient(kclient.NewFiltered[*v1alpha1.GatewayExtension](
		client,
		kclient.Filter{ObjectFilter: client.ObjectFilter()},
	), krtOpts.ToOptions("GatewayExtension")...)
	gwExtCol := krt.NewCollection(rawGwExts, func(krtctx krt.HandlerContext, cr *v1alpha1.GatewayExtension) *ir.GatewayExtension {
		gwExt := &ir.GatewayExtension{
			ObjectSource: ir.ObjectSource{
				Group:     wellknown.GatewayExtensionGVK.GroupKind().Group,
				Kind:      wellknown.GatewayExtensionGVK.GroupKind().Kind,
				Namespace: cr.Namespace,
				Name:      cr.Name,
			},
			Type:      cr.Spec.Type,
			ExtAuth:   cr.Spec.ExtAuth,
			ExtProc:   cr.Spec.ExtProc,
			RateLimit: cr.Spec.RateLimit,
		}
		return gwExt
	})
	return gwExtCol
}
