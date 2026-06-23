package reports

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestBuildGWStatusDoesNotMutateReportMapEntry(t *testing.T) {
	rm := NewReportMap()
	rep := NewReporter(&rm)

	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "gw",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{{
				Name:     "http",
				Port:     80,
				Protocol: gwv1.HTTPProtocolType,
			}},
		},
	}

	rep.Gateway(gw).Listener(&gw.Spec.Listeners[0])
	gr := rm.GatewayNamespaceName(key(gw))
	require.NotNil(t, gr)
	beforeCond := append([]metav1.Condition(nil), gr.conditions...)
	beforeListenerCond := append([]metav1.Condition(nil), gr.listeners["http"].Status.Conditions...)

	status := rm.BuildGWStatus(context.Background(), *gw, nil)
	require.NotNil(t, status)

	afterCond := append([]metav1.Condition(nil), gr.conditions...)
	afterListenerCond := append([]metav1.Condition(nil), gr.listeners["http"].Status.Conditions...)
	require.Equal(t, beforeCond, afterCond, "BuildGWStatus must not mutate GatewayReport conditions")
	require.Equal(t, beforeListenerCond, afterListenerCond, "BuildGWStatus must not mutate ListenerReport conditions")
}

func TestBuildListenerSetStatusDoesNotMutateReportMapEntry(t *testing.T) {
	rm := NewReportMap()
	rep := NewReporter(&rm)

	ls := &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "ls",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: gwv1.ListenerSetSpec{
			Listeners: []gwv1.ListenerEntry{{
				Name:     "http",
				Port:     80,
				Protocol: gwv1.HTTPProtocolType,
			}},
		},
	}
	ls.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gwv1.GroupVersion.Group,
		Version: gwv1.GroupVersion.Version,
		Kind:    "ListenerSet",
	})

	rep.ListenerSet(ls).ListenerName("http")
	lsr := rm.ListenerSet(ls)
	require.NotNil(t, lsr)
	beforeCond := append([]metav1.Condition(nil), lsr.conditions...)
	beforeListenerCond := append([]metav1.Condition(nil), lsr.listeners["http"].Status.Conditions...)

	status := rm.BuildListenerSetStatus(context.Background(), *ls)
	require.NotNil(t, status)

	afterCond := append([]metav1.Condition(nil), lsr.conditions...)
	afterListenerCond := append([]metav1.Condition(nil), lsr.listeners["http"].Status.Conditions...)
	require.Equal(t, beforeCond, afterCond, "BuildListenerSetStatus must not mutate ListenerSetReport conditions")
	require.Equal(t, beforeListenerCond, afterListenerCond, "BuildListenerSetStatus must not mutate listener conditions")
}
