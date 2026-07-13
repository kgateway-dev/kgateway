package proxy_syncer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	plug "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	reportssdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestBuildPolicyStatusPrefersPluginBuilder(t *testing.T) {
	key := reportssdk.PolicyKey{
		Group:     gwv1.GroupVersion.Group,
		Kind:      "BackendTLSPolicy",
		Namespace: "default",
		Name:      "tls-policy",
	}
	ancestorRef := gwv1.ParentReference{
		Group:     ptrTo(gwv1.Group(gwv1.GroupVersion.Group)),
		Kind:      ptrTo(gwv1.Kind("Gateway")),
		Namespace: ptrTo(gwv1.Namespace("default")),
		Name:      gwv1.ObjectName("gw"),
	}

	rm := reports.NewReportMap()
	reports.NewReporter(&rm).Policy(key, 1).AncestorRef(ancestorRef).SetCondition(reportssdk.PolicyCondition{
		Type:               string(gwv1.PolicyConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(gwv1.PolicyReasonAccepted),
		ObservedGeneration: 1,
	})

	pluginCalled := false
	plugin := plug.PolicyPlugin{
		BuildPolicyStatus: func(
			_ context.Context,
			reportMap reports.ReportMap,
			reportKey reportssdk.PolicyKey,
			_ string,
			_ gwv1.PolicyStatus,
		) *gwv1.PolicyStatus {
			pluginCalled = true
			report := reportMap.Policies[reportKey]
			require.NotNil(t, report)
			for _, ancestor := range report.Ancestors {
				require.Nil(t,
					apimeta.FindStatusCondition(ancestor.Conditions, string(shared.PolicyConditionAttached)),
					"plugin-specific status builder should not see a synthetic Attached condition",
				)
			}
			return &gwv1.PolicyStatus{}
		},
	}

	status := buildPolicyStatus(context.Background(), rm, plugin, key, "kgateway.dev/kgateway", gwv1.PolicyStatus{})
	require.True(t, pluginCalled)
	require.NotNil(t, status)

	report := rm.Policies[key]
	require.NotNil(t, report)
	for _, ancestor := range report.Ancestors {
		require.Nil(t,
			apimeta.FindStatusCondition(ancestor.Conditions, string(shared.PolicyConditionAttached)),
			"plugin-specific policy status selection should not mutate the report map",
		)
	}
}

//go:fix inline
func ptrTo[T any](v T) *T {
	return new(v)
}
