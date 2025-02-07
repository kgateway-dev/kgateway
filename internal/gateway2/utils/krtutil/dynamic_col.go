package krtutil

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/solo-io/go-utils/contextutils"
)

// SetupCollectionDynamic uses the dynamic client to setup an informer for a resource
// and then uses an intermediate krt collection to type the unstructured resource.
// This is a temporary workaround until we update to the latest istio version and can
// uncomment the code below for registering types.
// HACK: we don't want to use this long term, but it's letting me push forward with deveopment
func SetupCollectionDynamic[T any](
	ctx context.Context,
	client kube.Client,
	gvr schema.GroupVersionResource,
	opts ...krt.CollectionOption,
) krt.Collection[*T] {
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("setting up dynamic collection for %s", gvr.String())
	var dummy T
	typeName := fmt.Sprintf("dyn_%T", dummy)
	delayedClient := kclient.NewDelayedInformer[*unstructured.Unstructured](client, gvr, kubetypes.DynamicInformer, kclient.Filter{
		ObjectTransform: func(obj any) (any, error) {
			t, ok := obj.(metav1.ObjectMetaAccessor)
			if !ok {
				// shouldn't happen
				return obj, nil
			}
			// ManagedFields is large and we never use it
			t.GetObjectMeta().SetManagedFields(nil)
			return obj, nil
		},
	})
	mapper := krt.WrapClient(delayedClient, krt.WithName(typeName))
	return krt.NewCollection(mapper, func(krtctx krt.HandlerContext, i *unstructured.Unstructured) **T {
		var empty T
		out := &empty
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(i.UnstructuredContent(), out)
		if err != nil {
			logger.DPanic("failed converting unstructured into %T: %v", empty, i)
			return nil
		}
		return &out
	}, opts...)
}
