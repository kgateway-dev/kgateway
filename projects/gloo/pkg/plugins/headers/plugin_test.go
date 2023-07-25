package headers

import (
	"os"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/api_conversion"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	coreV1 "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
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
	DescribeTable("Invalid headers", func(key string, value string, expectedErr error) {
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
		Expect(err).To(MatchError(expectedErr))
	},
		Entry("Can't set pseudo-header", ":scheme", "value", CantSetPseudoHeaderError(":scheme")),
		Entry("Can't set Host header (Host)", "Host", "value", CantSetHostHeaderError),
		Entry("Can't set Host header (host)", "host", "value", CantSetHostHeaderError),
		Entry("Can't set Host header (HOST)", "HOST", "value", CantSetHostHeaderError),
		Entry("Can't set Host header (hOST)", "hOST", "value", CantSetHostHeaderError),
	)
	Context("Require secrets to match upstream namespace ", func() {
		BeforeEach(func() {
			os.Setenv(api_conversion.MatchingNamespaceEnv, "true")
			p = NewPlugin()
		})
		paramsWithSecret := plugins.Params{
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
		}
		singleRouteToBadUpstream := &v1.Route{Action: &v1.Route_RouteAction{RouteAction: &v1.RouteAction{
			Destination: &v1.RouteAction_Single{Single: &v1.Destination{DestinationType: &v1.Destination_Upstream{Upstream: &coreV1.ResourceRef{Name: "some-us", Namespace: "bad-tenant"}}}}}}}
		singleRouteToGoodUpstream := &v1.Route{Action: &v1.Route_RouteAction{RouteAction: &v1.RouteAction{
			Destination: &v1.RouteAction_Single{Single: &v1.Destination{DestinationType: &v1.Destination_Upstream{Upstream: &coreV1.ResourceRef{Name: "some-us", Namespace: "bar"}}}}}}}
		It("Errors with a WeightedDestination where the upstream namespace doesn't match the secret", func() {
			routeParamsWithSecret := plugins.RouteParams{VirtualHostParams: plugins.VirtualHostParams{Params: paramsWithSecret}}
			out := &envoy_config_route_v3.WeightedCluster_ClusterWeight{}
			in := &v1.WeightedDestination{
				Options:     &v1.WeightedDestinationOptions{HeaderManipulation: testHeaderManipWithSecrets},
				Destination: &v1.Destination{DestinationType: &v1.Destination_Upstream{Upstream: &coreV1.ResourceRef{Name: "some-us", Namespace: "bad-tenant"}}}}
			err := p.ProcessWeightedDestination(routeParamsWithSecret, in, out)
			Expect(err).To(MatchError("list did not find secret bar.foo"))
		})
		It("Errors with a VirtualHost with routes to one upstream that matches the secret and one that doesn't", func() {
			vhostParamsWithSecret := plugins.VirtualHostParams{
				Params: paramsWithSecret,
			}

			out := &envoy_config_route_v3.VirtualHost{}
			err := p.ProcessVirtualHost(vhostParamsWithSecret, &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					HeaderManipulation: testHeaderManipWithSecrets,
				},
				Routes: []*v1.Route{singleRouteToBadUpstream, singleRouteToGoodUpstream},
			}, out)
			Expect(err).To(HaveOccurred())
			Expect(err).Should(MatchError("list did not find secret bar.foo"))
		})
		It("Does not error with a VirtualHost with routes to multiple upstreams when secrets are not added to headers", func() {
			vhostParamsWithSecret := plugins.VirtualHostParams{
				Params: paramsWithSecret,
			}

			out := &envoy_config_route_v3.VirtualHost{}
			err := p.ProcessVirtualHost(vhostParamsWithSecret, &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					HeaderManipulation: testHeaderManip,
				},
				Routes: []*v1.Route{singleRouteToGoodUpstream, singleRouteToBadUpstream},
			}, out)
			Expect(err).NotTo(HaveOccurred())
		})
		It("Does not error with a VirtualHost with a route to an upstream that matches the secret", func() {
			vhostParamsWithSecret := plugins.VirtualHostParams{
				Params: paramsWithSecret,
			}

			out := &envoy_config_route_v3.VirtualHost{}
			err := p.ProcessVirtualHost(vhostParamsWithSecret, &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					HeaderManipulation: testHeaderManipWithSecrets,
				},
				Routes: []*v1.Route{singleRouteToGoodUpstream},
			}, out)
			Expect(err).NotTo(HaveOccurred())
		})
		It("Errors with a Route to an upstream that does not match the secret", func() {
			routeParamsWithSecret := plugins.RouteParams{VirtualHostParams: plugins.VirtualHostParams{Params: paramsWithSecret}}
			out := &envoy_config_route_v3.Route{}
			in := &v1.Route{Action: singleRouteToBadUpstream.GetAction(), Options: &v1.RouteOptions{HeaderManipulation: testHeaderManipWithSecrets}}
			err := p.ProcessRoute(routeParamsWithSecret, in, out)
			Expect(err).To(MatchError("list did not find secret bar.foo"))
		})
		It("Does not error when Route destination matches secret", func() {
			routeParamsWithSecret := plugins.RouteParams{VirtualHostParams: plugins.VirtualHostParams{Params: paramsWithSecret}}
			out := &envoy_config_route_v3.Route{}
			in := &v1.Route{Action: singleRouteToGoodUpstream.GetAction(), Options: &v1.RouteOptions{HeaderManipulation: testHeaderManipWithSecrets}}
			err := p.ProcessRoute(routeParamsWithSecret, in, out)
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			os.Clearenv()
			p = NewPlugin()
		})
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
