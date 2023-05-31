package headers

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayV1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	coreV1 "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	ns = "gloo-system"
)

var _ = Describe("Plugin", func() {
	p := NewPlugin()
	It("errors if the request header is nil", func() {
		out := &envoy_config_route_v3.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				HeaderManipulation: testBrokenConfigNoRequestHeader,
			},
		}, out)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("Unexpected header option type <nil>"))
	})
	It("errors if the response header is nil", func() {
		out := &envoy_config_route_v3.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				HeaderManipulation: testBrokenConfigNoResponseHeader,
			},
		}, out)
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(MissingHeaderValueError))
	})
	It("converts the header manipulation config for weighted destinations", func() {
		out := &envoy_config_route_v3.WeightedCluster_ClusterWeight{}
		err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
			Options: &v1.WeightedDestinationOptions{
				HeaderManipulation: testHeaderManip,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.RequestHeadersToAdd).To(Equal(expectedHeaders.RequestHeadersToAdd))
		Expect(out.RequestHeadersToRemove).To(Equal(expectedHeaders.RequestHeadersToRemove))
		Expect(out.ResponseHeadersToAdd).To(Equal(expectedHeaders.ResponseHeadersToAdd))
		Expect(out.ResponseHeadersToRemove).To(Equal(expectedHeaders.ResponseHeadersToRemove))
	})
	It("converts the header manipulation config for virtual hosts", func() {
		out := &envoy_config_route_v3.VirtualHost{}
		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				HeaderManipulation: testHeaderManip,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.RequestHeadersToAdd).To(Equal(expectedHeaders.RequestHeadersToAdd))
		Expect(out.RequestHeadersToRemove).To(Equal(expectedHeaders.RequestHeadersToRemove))
		Expect(out.ResponseHeadersToAdd).To(Equal(expectedHeaders.ResponseHeadersToAdd))
		Expect(out.ResponseHeadersToRemove).To(Equal(expectedHeaders.ResponseHeadersToRemove))
	})
	It("converts the header manipulation config for routes", func() {
		out := &envoy_config_route_v3.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				HeaderManipulation: testHeaderManip,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.RequestHeadersToAdd).To(Equal(expectedHeaders.RequestHeadersToAdd))
		Expect(out.RequestHeadersToRemove).To(Equal(expectedHeaders.RequestHeadersToRemove))
		Expect(out.ResponseHeadersToAdd).To(Equal(expectedHeaders.ResponseHeadersToAdd))
		Expect(out.ResponseHeadersToRemove).To(Equal(expectedHeaders.ResponseHeadersToRemove))
	})
	It("Can add secrets to headers", func() {
		paramsWithSecret := plugins.VirtualHostParams{
			Params: plugins.Params{
				Snapshot: &v1snap.ApiSnapshot{
					Secrets: v1.SecretList{
						{
							Kind: &v1.Secret_Header{
								Header: &v1.HeaderSecret{
									Headers: map[string]string{
										"Authorization": "basic dXNlcjpwYXNzd29yZA==",
									},
								},
							},
							Metadata: &coreV1.Metadata{
								Name:      "foo",
								Namespace: "bar",
							},
						},
					},
				},
			},
		}

		out := &envoy_config_route_v3.VirtualHost{}
		err := p.ProcessVirtualHost(paramsWithSecret, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				HeaderManipulation: testHeaderManipWithSecrets,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.RequestHeadersToAdd).To(Equal(expectedHeadersWithSecrets.RequestHeadersToAdd))
		Expect(out.RequestHeadersToRemove).To(Equal(expectedHeadersWithSecrets.RequestHeadersToRemove))
		Expect(out.ResponseHeadersToAdd).To(Equal(expectedHeadersWithSecrets.ResponseHeadersToAdd))
		Expect(out.ResponseHeadersToRemove).To(Equal(expectedHeadersWithSecrets.ResponseHeadersToRemove))
	})

	It("Handles :schema header", func() {
		params := plugins.VirtualHostParams{
			Params: plugins.Params{
				Snapshot: &v1snap.ApiSnapshot{
					Gateways: gatewayV1.GatewayList{
						{
							Metadata: &coreV1.Metadata{Namespace: ns, Name: "gw1"},
							BindPort: 1111,
							GatewayType: &gatewayV1.Gateway_HttpGateway{
								HttpGateway: &gatewayV1.HttpGateway{},
							},
						},
					},
				},
			},
		}

		schemeHeader := headers.HeaderManipulation{
			ResponseHeadersToAdd: []*headers.HeaderValueOption{
				{
					Header: &headers.HeaderValue{
						Key:   ":scheme",
						Value: "httpssl",
					},
					Append: &wrappers.BoolValue{Value: true},
				},
			},
		}

		out := &envoy_config_route_v3.VirtualHost{}
		err := p.ProcessVirtualHost(params, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				HeaderManipulation: &schemeHeader,
			},
		}, out)
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(InvalidSchemeError))
		// Expect(out.RequestHeadersToAdd).To(Equal(expectedHeadersWithSecrets.RequestHeadersToAdd))
		// Expect(out.RequestHeadersToRemove).To(Equal(expectedHeadersWithSecrets.RequestHeadersToRemove))
		// Expect(out.ResponseHeadersToAdd).To(Equal(expectedHeadersWithSecrets.ResponseHeadersToAdd))
		// Expect(out.ResponseHeadersToRemove).To(Equal(expectedHeadersWithSecrets.ResponseHeadersToRemove))
	})

	FDescribeTable("Invalid headers", func(key string, value string, expectedErr error) {
		params := plugins.VirtualHostParams{}
		schemeHeader := headers.HeaderManipulation{
			ResponseHeadersToAdd: []*headers.HeaderValueOption{
				{
					Header: &headers.HeaderValue{
						Key:   key,
						Value: value,
					},
					Append: &wrappers.BoolValue{Value: true},
				},
			},
		}

		out := &envoy_config_route_v3.VirtualHost{}
		err := p.ProcessVirtualHost(params, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				HeaderManipulation: &schemeHeader,
			},
		}, out)
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(expectedErr))
	},
		Entry("invalid :scheme header value", ":scheme", "httpssl", InvalidSchemeError),
		Entry("invalid use of pseudo-header", ":not-schema", "value", CantSetPseudoHeaderError),
	)

	It("Valid :scheme header", func() {
		params := plugins.VirtualHostParams{}
		schemeHeader := headers.HeaderManipulation{
			ResponseHeadersToAdd: []*headers.HeaderValueOption{
				{
					Header: &headers.HeaderValue{
						Key:   ":scheme",
						Value: "http",
					},
					Append: &wrappers.BoolValue{Value: true},
				},
			},
		}

		out := &envoy_config_route_v3.VirtualHost{}
		err := p.ProcessVirtualHost(params, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				HeaderManipulation: &schemeHeader,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.RequestHeadersToAdd).To(Equal(expectedHeadersWithSecrets.RequestHeadersToAdd))
		Expect(out.RequestHeadersToRemove).To(Equal(expectedHeadersWithSecrets.RequestHeadersToRemove))
		Expect(out.ResponseHeadersToAdd).To(Equal(expectedHeadersWithSecrets.ResponseHeadersToAdd))
		Expect(out.ResponseHeadersToRemove).To(Equal(expectedHeadersWithSecrets.ResponseHeadersToRemove))
	})
})

var testBrokenConfigNoRequestHeader = &headers.HeaderManipulation{
	RequestHeadersToAdd:     []*envoycore_sk.HeaderValueOption{{HeaderOption: nil, Append: &wrappers.BoolValue{Value: true}}},
	RequestHeadersToRemove:  []string{"a"},
	ResponseHeadersToAdd:    []*headers.HeaderValueOption{{Header: &headers.HeaderValue{Key: "foo", Value: "bar"}, Append: &wrappers.BoolValue{Value: true}}},
	ResponseHeadersToRemove: []string{"b"},
}

var testBrokenConfigNoResponseHeader = &headers.HeaderManipulation{
	RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{{HeaderOption: &envoycore_sk.HeaderValueOption_Header{Header: &envoycore_sk.HeaderValue{Key: "foo", Value: "bar"}},
		Append: &wrappers.BoolValue{Value: true}}},
	RequestHeadersToRemove:  []string{"a"},
	ResponseHeadersToAdd:    []*headers.HeaderValueOption{{Header: nil, Append: &wrappers.BoolValue{Value: true}}},
	ResponseHeadersToRemove: []string{"b"},
}

var testHeaderManip = &headers.HeaderManipulation{
	RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{{HeaderOption: &envoycore_sk.HeaderValueOption_Header{Header: &envoycore_sk.HeaderValue{Key: "foo", Value: "bar"}},
		Append: &wrappers.BoolValue{Value: true}}},
	RequestHeadersToRemove:  []string{"a"},
	ResponseHeadersToAdd:    []*headers.HeaderValueOption{{Header: &headers.HeaderValue{Key: "foo", Value: "bar"}, Append: &wrappers.BoolValue{Value: true}}},
	ResponseHeadersToRemove: []string{"b"},
}

var expectedHeaders = envoyHeaderManipulation{
	RequestHeadersToAdd:     []*envoy_config_core_v3.HeaderValueOption{{Header: &envoy_config_core_v3.HeaderValue{Key: "foo", Value: "bar"}, Append: &wrappers.BoolValue{Value: true}}},
	RequestHeadersToRemove:  []string{"a"},
	ResponseHeadersToAdd:    []*envoy_config_core_v3.HeaderValueOption{{Header: &envoy_config_core_v3.HeaderValue{Key: "foo", Value: "bar"}, Append: &wrappers.BoolValue{Value: true}}},
	ResponseHeadersToRemove: []string{"b"},
}

var testHeaderManipWithSecrets = &headers.HeaderManipulation{
	RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{{HeaderOption: &envoycore_sk.HeaderValueOption_HeaderSecretRef{HeaderSecretRef: &coreV1.ResourceRef{Name: "foo", Namespace: "bar"}},
		Append: &wrappers.BoolValue{Value: true}}},
	RequestHeadersToRemove:  []string{"a"},
	ResponseHeadersToAdd:    []*headers.HeaderValueOption{{Header: &headers.HeaderValue{Key: "foo", Value: "bar"}, Append: &wrappers.BoolValue{Value: true}}},
	ResponseHeadersToRemove: []string{"b"},
}

var expectedHeadersWithSecrets = envoyHeaderManipulation{
	RequestHeadersToAdd:     []*envoy_config_core_v3.HeaderValueOption{{Header: &envoy_config_core_v3.HeaderValue{Key: "Authorization", Value: "basic dXNlcjpwYXNzd29yZA=="}, Append: &wrappers.BoolValue{Value: true}}},
	RequestHeadersToRemove:  []string{"a"},
	ResponseHeadersToAdd:    []*envoy_config_core_v3.HeaderValueOption{{Header: &envoy_config_core_v3.HeaderValue{Key: "foo", Value: "bar"}, Append: &wrappers.BoolValue{Value: true}}},
	ResponseHeadersToRemove: []string{"b"},
}
