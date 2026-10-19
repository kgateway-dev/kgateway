package irtranslator

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	reportssdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// listenerSetGatewayIR builds a Gateway with one listener of its own followed by
// one HTTPS listener in each of n ListenerSets, and returns the name of the filter
// chain each listener owns.
func listenerSetGatewayIR(n int) (ir.GatewayIR, []string) {
	gw := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "gw"}}
	src := &ir.Gateway{Obj: gw}
	src.Listeners = append(src.Listeners, ir.Listener{Parent: gw, Listener: gwv1.Listener{Name: "https"}})
	for i := range n {
		ls := &gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: fmt.Sprintf("ls-%d", i)}}
		src.Listeners = append(src.Listeners, ir.Listener{Parent: ls, Listener: gwv1.Listener{Name: "https"}})
	}
	names := make([]string, 0, len(src.Listeners))
	for _, l := range src.Listeners {
		names = append(names, query.GenerateRouteKey(l.Parent, string(l.Name)))
	}
	return ir.GatewayIR{SourceObject: src}, names
}

func TestFilterChainListeners(t *testing.T) {
	gw, chainNames := listenerSetGatewayIR(3)
	// Repeat the names of the Gateway's own listener and of ls-0's listener after
	// them; a scan in listener order finds the first of each.
	src := gw.SourceObject
	src.Listeners = append(src.Listeners,
		ir.Listener{Parent: src.Obj, Listener: gwv1.Listener{Name: "https", Port: 8443}},
		ir.Listener{Parent: src.Listeners[1].Parent, Listener: gwv1.Listener{Name: "https", Port: 8443}},
	)
	listeners := src.Listeners
	chainListeners := newFilterChainListeners(gw)

	tests := []struct {
		name            string
		filterChainName string
		want            *ir.Listener
	}{
		{name: "gateway listener", filterChainName: "https", want: &listeners[0]},
		{name: "first listener set listener", filterChainName: "default/ls-0/https", want: &listeners[1]},
		{name: "last listener set listener", filterChainName: "default/ls-2/https", want: &listeners[3]},
		{name: "merged plain http chain", filterChainName: "listener~80", want: nil},
		{name: "listener set in another namespace", filterChainName: "other/ls-0/https", want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Same(t, tc.want, chainListeners[tc.filterChainName])
		})
	}

	for i, name := range chainNames {
		assert.Same(t, &listeners[i], chainListeners[name], "chain %q should map to its own listener", name)
	}
}

// An HTTPS chain reports on the listener that owns it. A merged plain HTTP chain has
// no single owner, so it reports on a listener of the Gateway named for the chain.
func TestGetReporterForFilterChain(t *testing.T) {
	gw, _ := listenerSetGatewayIR(2)
	chainListeners := newFilterChainListeners(gw)
	rm := reports.NewReportMap()
	r := reports.NewReporter(&rm)
	for _, name := range []string{"default/ls-1/https", "listener~80"} {
		getReporterForFilterChain(gw, r, chainListeners, name).SetCondition(reportssdk.ListenerCondition{
			Type:   gwv1.ListenerConditionAccepted,
			Status: metav1.ConditionFalse,
			Reason: reportssdk.ListenerReplacedReason,
		})
	}

	accepted := func(lr reportssdk.ListenerReporter) *metav1.Condition {
		return meta.FindStatusCondition(lr.(*reports.ListenerReport).Status.Conditions, string(gwv1.ListenerConditionAccepted))
	}
	lsReport := rm.ListenerSet(gw.SourceObject.Listeners[2].Parent)
	require.NotNil(t, lsReport, "the ListenerSet that owns the HTTPS chain must have a report")
	assert.NotNil(t, accepted(lsReport.ListenerName("https")), "the HTTPS chain should report on its own listener")
	assert.Nil(t, rm.ListenerSet(gw.SourceObject.Listeners[1].Parent), "other ListenerSets must not get a report")

	gwReport := rm.Gateway(gw.SourceObject.Obj)
	require.NotNil(t, gwReport, "the merged plain HTTP chain should fall back to the Gateway")
	assert.NotNil(t, accepted(gwReport.ListenerName("listener~80")), "the fallback should name the chain")
	assert.Nil(t, accepted(gwReport.ListenerName("https")), "the Gateway's own HTTPS listener must not get the condition")
}
