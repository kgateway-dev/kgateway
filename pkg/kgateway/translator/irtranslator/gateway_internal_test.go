package irtranslator

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
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

// BenchmarkFindListenerForFilterChain does the lookups of one gateway translation:
// one per HTTPS filter chain, plus a merged plain HTTP chain that matches nothing.
//
// Run with:
//
//	go test -run=^$ -bench=BenchmarkFindListenerForFilterChain -benchmem \
//	    ./pkg/kgateway/translator/irtranslator/
func BenchmarkFindListenerForFilterChain(b *testing.B) {
	gw, chainNames := listenerSetGatewayIR(500)
	chainNames = append(chainNames, "listener~80")
	b.ReportAllocs()
	for b.Loop() {
		for _, name := range chainNames {
			findListenerForFilterChain(gw, name)
		}
	}
}

func TestFindListenerForFilterChain(t *testing.T) {
	gw, chainNames := listenerSetGatewayIR(3)
	listeners := gw.SourceObject.Listeners

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
			assert.Same(t, tc.want, findListenerForFilterChain(gw, tc.filterChainName))
		})
	}

	for i, name := range chainNames {
		assert.Same(t, &listeners[i], findListenerForFilterChain(gw, name), "chain %q should map to its own listener", name)
	}
}
