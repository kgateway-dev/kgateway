package proxy_syncer

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestSyncListenerSetStatusSkipsUnsupportedKinds(t *testing.T) {
	for _, tc := range []struct {
		name      string
		supported bool
		sameName  bool
	}{
		{name: "custom only"},
		{name: "mixed supported and custom", supported: true},
		{name: "same namespace and name across kinds", supported: true, sameName: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := runtime.NewScheme()
			require.NoError(t, gwv1.Install(scheme))
			rm := reports.NewReportMap()
			statusReporter := reports.NewReporter(&rm)
			custom := &unstructured.Unstructured{}
			custom.SetGroupVersionKind(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "CustomListenerSet"})
			custom.SetNamespace("default")
			custom.SetName("custom")
			if tc.sameName {
				custom.SetName("shared")
			}
			statusReporter.ListenerSet(custom).SetCondition(reporter.GatewayCondition{
				Type: gwv1.GatewayConditionAccepted, Status: metav1.ConditionFalse,
				Reason: gwv1.GatewayReasonListenersNotValid, Message: "custom rejection",
			})
			customReport := rm.ListenerSet(custom)
			customConditions := append([]metav1.Condition(nil), customReport.GetConditions()...)

			var objects []ctrlclient.Object
			if tc.supported {
				for _, gvk := range []schema.GroupVersionKind{wellknown.ListenerSetGVK, wellknown.XListenerSetGVK} {
					ls := &gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "shared", Generation: 3}}
					ls.SetGroupVersionKind(gvk)
					statusReporter.ListenerSet(ls).SetCondition(reporter.GatewayCondition{
						Type: gwv1.GatewayConditionAccepted, Status: metav1.ConditionTrue,
						Reason: gwv1.GatewayReasonAccepted, Message: "supported accepted",
					})
					if gvk == wellknown.XListenerSetGVK {
						content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(ls)
						require.NoError(t, err)
						objects = append(objects, &unstructured.Unstructured{Object: content})
					} else {
						objects = append(objects, ls)
					}
				}
			}
			reads := map[schema.GroupVersionKind]int{}
			writes := map[schema.GroupVersionKind]int{}
			kubeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(objects...).WithStatusSubresource(objects...).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, cl ctrlclient.WithWatch, key ctrlclient.ObjectKey, obj ctrlclient.Object, opts ...ctrlclient.GetOption) error {
						gvk, err := cl.GroupVersionKindFor(obj)
						require.NoError(t, err)
						reads[gvk]++
						return cl.Get(ctx, key, obj, opts...)
					},
					SubResourcePatch: func(ctx context.Context, cl ctrlclient.Client, subresource string, obj ctrlclient.Object, patch ctrlclient.Patch, opts ...ctrlclient.SubResourcePatchOption) error {
						require.Equal(t, "status", subresource)
						gvk, err := cl.GroupVersionKindFor(obj)
						require.NoError(t, err)
						writes[gvk]++
						return cl.SubResource(subresource).Patch(ctx, obj, patch, opts...)
					},
				}).Build()
			syncer := &StatusSyncer{mgr: statusSyncerTestManager{client: kubeClient}}
			var logs bytes.Buffer
			syncer.syncListenerSetStatus(ctx, slog.New(slog.NewTextHandler(&logs, nil)), rm)

			expected := map[schema.GroupVersionKind]int{}
			if tc.supported {
				expected[wellknown.ListenerSetGVK] = 1
				expected[wellknown.XListenerSetGVK] = 1
			}
			require.Equal(t, expected, reads, "only supported resources should be fetched, exactly once")
			require.Equal(t, expected, writes, "only supported resources should receive status writes")
			require.NotContains(t, logs.String(), "error getting ls")
			require.NotContains(t, logs.String(), "all attempts failed")
			require.Same(t, customReport, rm.ListenerSet(custom), "retain the report for the custom writer")
			require.Equal(t, customConditions, customReport.GetConditions())
			for _, obj := range objects {
				require.NoError(t, kubeClient.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), obj))
				content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
				require.NoError(t, err)
				var updated gwv1.ListenerSet
				require.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(content, &updated))
				accepted := apimeta.FindStatusCondition(updated.Status.Conditions, string(gwv1.ListenerSetConditionAccepted))
				require.NotNil(t, accepted)
				require.Equal(t, metav1.ConditionTrue, accepted.Status)
				require.Equal(t, "supported accepted", accepted.Message)
				require.Equal(t, int64(3), accepted.ObservedGeneration)
			}
		})
	}
}

func TestSyncListenerSetStatusRetriesMissingSupportedKinds(t *testing.T) {
	for _, gvk := range []schema.GroupVersionKind{wellknown.ListenerSetGVK, wellknown.XListenerSetGVK} {
		t.Run(gvk.Kind, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, gwv1.Install(scheme))
			reads := 0
			kubeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, cl ctrlclient.WithWatch, key ctrlclient.ObjectKey, obj ctrlclient.Object, opts ...ctrlclient.GetOption) error {
						reads++
						return cl.Get(ctx, key, obj, opts...)
					},
				}).Build()
			rm := reports.NewReportMap()
			rm.ListenerSets[gvk] = map[types.NamespacedName]*reports.ListenerSetReport{
				{Namespace: "default", Name: "missing"}: {},
			}
			syncer := &StatusSyncer{mgr: statusSyncerTestManager{client: kubeClient}}
			var logs bytes.Buffer
			syncer.syncListenerSetStatus(context.Background(), slog.New(slog.NewTextHandler(&logs, nil)), rm)
			require.Equal(t, 5, reads)
			require.Contains(t, logs.String(), "error getting ls")
			require.Contains(t, logs.String(), "all attempts failed at updating listener set statuses")
		})
	}
}
