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
		b.Run(fmt.Sprintf("endpoints=%d/replacement-builder", endpointCount), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				resolver := NewEndpointInputsResolver(base)
				replacement := resolver.NewEndpointSet()
				resolver.ForEachEndpoint(func(locality ir.PodLocality, endpoint EndpointView) bool {
					replacement.AddUnchanged(locality, endpoint)
					return true
				})
				resolver.ReplaceEndpoints(replacement)
				endpointEditorBenchSink = resolver.Inputs()
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

