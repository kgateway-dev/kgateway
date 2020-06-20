package tracing

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tracing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {

	It("should update listener properly", func() {
		p := NewPlugin()
		cfg := &envoyhttp.HttpConnectionManager{}
		hcmSettings := &hcm.HttpConnectionManagerSettings{
			Tracing: &tracing.ListenerTracingSettings{
				RequestHeadersForTags: []string{"header1", "header2"},
				Verbose:               true,
				TracePercentages: &tracing.TracePercentages{
					ClientSamplePercentage:  &types.FloatValue{Value: 10},
					RandomSamplePercentage:  &types.FloatValue{Value: 20},
					OverallSamplePercentage: &types.FloatValue{Value: 30},
				},
			},
		}
		err := p.ProcessHcmSettings(cfg, hcmSettings)
		Expect(err).To(BeNil())
		expected := &envoyhttp.HttpConnectionManager{
			Tracing: &envoyhttp.HttpConnectionManager_Tracing{
				HiddenEnvoyDeprecatedOperationName:         envoyhttp.HttpConnectionManager_Tracing_INGRESS,
				HiddenEnvoyDeprecatedRequestHeadersForTags: []string{"header1", "header2"},
				ClientSampling:  &envoy_type.Percent{Value: 10},
				RandomSampling:  &envoy_type.Percent{Value: 20},
				OverallSampling: &envoy_type.Percent{Value: 30},
				Verbose:         true,
			},
		}
		Expect(cfg).To(Equal(expected))
	})

	It("should update listener properly - with defaults", func() {
		p := NewPlugin()
		cfg := &envoyhttp.HttpConnectionManager{}
		hcmSettings := &hcm.HttpConnectionManagerSettings{
			Tracing: &tracing.ListenerTracingSettings{},
		}
		err := p.ProcessHcmSettings(cfg, hcmSettings)
		Expect(err).To(BeNil())
		expected := &envoyhttp.HttpConnectionManager{
			Tracing: &envoyhttp.HttpConnectionManager_Tracing{
				HiddenEnvoyDeprecatedOperationName: envoyhttp.HttpConnectionManager_Tracing_INGRESS,
				ClientSampling:                     &envoy_type.Percent{Value: 100},
				RandomSampling:                     &envoy_type.Percent{Value: 100},
				OverallSampling:                    &envoy_type.Percent{Value: 100},
				Verbose:                            false,
			},
		}
		Expect(cfg).To(Equal(expected))
	})

	It("should update routes properly", func() {
		p := NewPlugin()
		in := &v1.Route{}
		out := &envoyroute.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).NotTo(HaveOccurred())

		inFull := &v1.Route{
			Options: &v1.RouteOptions{
				Tracing: &tracing.RouteTracingSettings{
					RouteDescriptor: "hello",
				},
			},
		}
		outFull := &envoyroute.Route{}
		err = p.ProcessRoute(plugins.RouteParams{}, inFull, outFull)
		Expect(err).NotTo(HaveOccurred())
		Expect(outFull.Decorator.Operation).To(Equal("hello"))
		Expect(outFull.Tracing.ClientSampling.Numerator / 10000).To(Equal(uint32(100)))
		Expect(outFull.Tracing.RandomSampling.Numerator / 10000).To(Equal(uint32(100)))
		Expect(outFull.Tracing.OverallSampling.Numerator / 10000).To(Equal(uint32(100)))
	})

	It("should update routes properly - with defaults", func() {
		p := NewPlugin()
		in := &v1.Route{}
		out := &envoyroute.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).NotTo(HaveOccurred())

		inFull := &v1.Route{
			Options: &v1.RouteOptions{
				Tracing: &tracing.RouteTracingSettings{
					RouteDescriptor: "hello",
					TracePercentages: &tracing.TracePercentages{
						ClientSamplePercentage:  &types.FloatValue{Value: 10},
						RandomSamplePercentage:  &types.FloatValue{Value: 20},
						OverallSamplePercentage: &types.FloatValue{Value: 30},
					},
				},
			},
		}
		outFull := &envoyroute.Route{}
		err = p.ProcessRoute(plugins.RouteParams{}, inFull, outFull)
		Expect(err).NotTo(HaveOccurred())
		Expect(outFull.Decorator.Operation).To(Equal("hello"))
		Expect(outFull.Tracing.ClientSampling.Numerator / 10000).To(Equal(uint32(10)))
		Expect(outFull.Tracing.RandomSampling.Numerator / 10000).To(Equal(uint32(20)))
		Expect(outFull.Tracing.OverallSampling.Numerator / 10000).To(Equal(uint32(30)))
	})

})
