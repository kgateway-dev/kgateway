package irtranslator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	reportssdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// A chain that records its listener reports on that listener, whether the Gateway or a
// ListenerSet owns it.
func TestGetReporterForFilterChain(t *testing.T) {
	gw := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "gw"}}
	ls := &gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "ls"}}
	gwIR := ir.GatewayIR{SourceObject: &ir.Gateway{Obj: gw}}
	rm := reports.NewReportMap()
	r := reports.NewReporter(&rm)
	for _, fc := range []ir.FilterChainCommon{
		{FilterChainName: "https", Listener: &ir.Listener{Parent: gw, Listener: gwv1.Listener{Name: "https"}}},
		{FilterChainName: "default/ls/https", Listener: &ir.Listener{Parent: ls, Listener: gwv1.Listener{Name: "https"}}},
	} {
		getReporterForFilterChain(gwIR, r, fc).SetCondition(reportssdk.ListenerCondition{
			Type:   gwv1.ListenerConditionAccepted,
			Status: metav1.ConditionFalse,
			Reason: reportssdk.ListenerReplacedReason,
		})
	}

	gwReport := rm.Gateway(gw)
	require.NotNil(t, gwReport, "the Gateway that owns the first chain must have a report")
	lsReport := rm.ListenerSet(ls)
	require.NotNil(t, lsReport, "the ListenerSet that owns the second chain must have a report")
	for owner, listener := range map[string]reportssdk.ListenerReporter{
		"Gateway":     gwReport.ListenerName("https"),
		"ListenerSet": lsReport.ListenerName("https"),
	} {
		condition := meta.FindStatusCondition(listener.(*reports.ListenerReport).Status.Conditions, string(gwv1.ListenerConditionAccepted))
		require.NotNil(t, condition, "the %s listener should get its chain's condition", owner)
		assert.Equal(t, string(reportssdk.ListenerReplacedReason), condition.Reason, "%s listener", owner)
	}
}
