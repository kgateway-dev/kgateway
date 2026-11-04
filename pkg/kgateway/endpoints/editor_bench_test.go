package endpoints

import (
	"fmt"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var endpointEditorBenchSink EndpointsInputs

func BenchmarkEndpointInputsResolver(b *testing.B) {
	for _, endpointCount := range []int{10, 100, 1000} {
		base := benchmarkEndpointInputs(endpointCount)
		b.Run(fmt.Sprintf("endpoints=%d/scalar-edit", endpointCount), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				resolver := NewEndpointInputsResolver(base)
				resolver.SetTrafficDistribution(wellknown.TrafficDistributionAny)
				endpointEditorBenchSink = resolver.Inputs()
			}
		})
		b.Run(fmt.Sprintf("endpoints=%d/replacement-all-unchanged", endpointCount), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				benchmarkReplacement(base, 0)
			}
		})
		b.Run(fmt.Sprintf("endpoints=%d/replacement-90pct-unchanged", endpointCount), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				benchmarkReplacement(base, 10)
			}
		})
		b.Run(fmt.Sprintf("endpoints=%d/replacement-50pct-unchanged", endpointCount), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				benchmarkReplacement(base, 2)
			}
		})
		b.Run(fmt.Sprintf("endpoints=%d/legacy-deep-copy", endpointCount), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				resolver := NewEndpointInputsResolver(base)
				resolver.LegacyMutableInputs()
				endpointEditorBenchSink = resolver.Inputs()
			}
		})
	}
}

// benchmarkReplacement models the peering endpoint editor: most endpoints are
// structurally shared while every modifyEvery-th endpoint is cloned and
// rewritten. A zero modifyEvery keeps every endpoint unchanged.
func benchmarkReplacement(base EndpointsInputs, modifyEvery int) {
	resolver := NewEndpointInputsResolver(base)
	replacement := resolver.NewEndpointSet()
	index := 0
	resolver.ForEachEndpoint(func(locality ir.PodLocality, endpoint EndpointView) bool {
		index++
		if modifyEvery == 0 || index%modifyEvery != 0 {
			replacement.AddUnchanged(locality, endpoint)
			return true
		}
		replacement.AddModified(locality, endpoint, func(ep *ir.EndpointWithMd) {
			ep.GetEndpoint().GetAddress().GetSocketAddress().Address = "127.167.0.1"
		})
		return true
	})
	resolver.ReplaceEndpoints(replacement)
	endpointEditorBenchSink = resolver.Inputs()
}

func benchmarkEndpointInputs(endpointCount int) EndpointsInputs {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 8080, "", "")
	backendEndpoints := ir.NewEndpointsForBackend(backend)
	for i := range endpointCount {
		backendEndpoints.Add(ir.PodLocality{Region: "r", Zone: fmt.Sprintf("z%d", i%3)}, editorTestEndpoint(
			fmt.Sprintf("10.0.%d.%d", i/255, i%255),
			fmt.Sprintf("endpoint-%d", i),
		))
	}
	return EndpointsInputs{EndpointsForBackend: *backendEndpoints}
}
