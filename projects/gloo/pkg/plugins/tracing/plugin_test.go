package tracing

import (
	"fmt"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoy_config_trace_v3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytracing "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/ghodss/yaml"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/ptypes/any"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tracing"
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
				CustomTags: []*envoytracing.CustomTag{
					{
						Tag: "header1",
						Type: &envoytracing.CustomTag_RequestHeader{
							RequestHeader: &envoytracing.CustomTag_Header{
								Name: "header1",
							},
						},
					},
					{
						Tag: "header2",
						Type: &envoytracing.CustomTag_RequestHeader{
							RequestHeader: &envoytracing.CustomTag_Header{
								Name: "header2",
							},
						},
					},
				},
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
				ClientSampling:  &envoy_type.Percent{Value: 100},
				RandomSampling:  &envoy_type.Percent{Value: 100},
				OverallSampling: &envoy_type.Percent{Value: 100},
				Verbose:         false,
				Provider:        nil,
			},
		}
		Expect(cfg).To(Equal(expected))
	})

	Context("should update provider", func() {

		It("when provider is nil", func() {
			p := NewPlugin()
			cfg := &envoyhttp.HttpConnectionManager{}
			hcmSettings := &hcm.HttpConnectionManagerSettings{
				Tracing: &tracing.ListenerTracingSettings{
					Provider: nil,
				},
			}
			err := p.ProcessHcmSettings(cfg, hcmSettings)
			Expect(err).To(BeNil())
			expected := &envoyhttp.HttpConnectionManager{
				Tracing: &envoyhttp.HttpConnectionManager_Tracing{
					Provider: nil,
				},
			}
			Expect(cfg.Tracing.Provider).To(Equal(expected.Tracing.Provider))
		})

		It("when provider is configured via yaml", func() {
			providerYaml := fmt.Sprint(`
---
name: envoy.tracers.zipkin
typed_config:
    "@type": "type.googleapis.com/envoy.config.trace.v3.ZipkinConfig"
    collector_cluster: zipkin
    collector_endpoint: "/api/v2/spans"
    collector_endpoint_version: HTTP_JSON
`)

			var provider tracing.Provider
			err := yaml.Unmarshal([]byte(providerYaml), &provider)
			Expect(err).NotTo(HaveOccurred())

			p := NewPlugin()
			cfg := &envoyhttp.HttpConnectionManager{}
			hcmSettings := &hcm.HttpConnectionManagerSettings{
				Tracing: &tracing.ListenerTracingSettings{
					Provider: &provider,
				},
			}

			expectedZipkinConfig := &envoy_config_trace_v3.ZipkinConfig{
				CollectorCluster:         "zipkin",
				CollectorEndpoint:        "/api/v2/spans",
				CollectorEndpointVersion: envoy_config_trace_v3.ZipkinConfig_HTTP_JSON,
			}
			serializedExpectedZipkinConfig, err := proto.Marshal(expectedZipkinConfig)

			err = p.ProcessHcmSettings(cfg, hcmSettings)
			Expect(err).NotTo(HaveOccurred())

			expected := &envoyhttp.HttpConnectionManager{
				Tracing: &envoyhttp.HttpConnectionManager_Tracing{
					Provider: &envoy_config_trace_v3.Tracing_Http{
						Name: "envoy.tracers.zipkin",
						ConfigType: &envoy_config_trace_v3.Tracing_Http_TypedConfig{
							TypedConfig: &any.Any{
								TypeUrl: "type.googleapis.com/envoy.config.trace.v3.ZipkinConfig",
								Value:   serializedExpectedZipkinConfig,
							},
						},
					},
				},
			}
			Expect(cfg.Tracing.Provider).To(Equal(expected.Tracing.Provider))
		})

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
