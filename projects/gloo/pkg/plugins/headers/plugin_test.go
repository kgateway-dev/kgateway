package headers

import (
	"errors"
	"os"

	"github.com/solo-io/gloo/pkg/utils/api_conversion"

	envoy_config_mutation_rules_v3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_ehm_header_mutation_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/early_header_mutation/header_mutation/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
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
		err := p.ProcessWeightedDestination(plugins.RouteActionParams{}, &v1.WeightedDestination{
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
	It("converts the header manipulation config for routes w/ overwrite", func() {
		out := &envoy_config_route_v3.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				HeaderManipulation: testHeaderManipOverwrite,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.RequestHeadersToAdd).To(Equal(expectedHeadersOverwrite.RequestHeadersToAdd))
		Expect(out.RequestHeadersToRemove).To(Equal(expectedHeadersOverwrite.RequestHeadersToRemove))
		Expect(out.ResponseHeadersToAdd).To(Equal(expectedHeadersOverwrite.ResponseHeadersToAdd))
		Expect(out.ResponseHeadersToRemove).To(Equal(expectedHeadersOverwrite.ResponseHeadersToRemove))
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
		var (
			singleRouteToBadUpstream  *v1.Route
			singleRouteToGoodUpstream *v1.Route
			weightedDestinationBadUs  *v1.Destination
			weightedDestinationGoodUs *v1.Destination
		)
		BeforeEach(func() {
			err := os.Setenv(api_conversion.MatchingNamespaceEnv, "true")
			Expect(err).NotTo(HaveOccurred(), "Error setting matching namespace environment variable")
			p = NewPlugin()
			singleRouteToBadUpstream = &v1.Route{Action: &v1.Route_RouteAction{RouteAction: &v1.RouteAction{
				Destination: &v1.RouteAction_Single{Single: &v1.Destination{DestinationType: &v1.Destination_Upstream{Upstream: &coreV1.ResourceRef{Name: "some-us", Namespace: "bad-tenant"}}}}}}}
			singleRouteToGoodUpstream = &v1.Route{Action: &v1.Route_RouteAction{RouteAction: &v1.RouteAction{
				Destination: &v1.RouteAction_Single{Single: &v1.Destination{DestinationType: &v1.Destination_Upstream{Upstream: &coreV1.ResourceRef{Name: "some-us", Namespace: "bar"}}}}}}}
			weightedDestinationBadUs = &v1.Destination{DestinationType: &v1.Destination_Upstream{Upstream: &coreV1.ResourceRef{Name: "some-us", Namespace: "bad-tenant"}}}
			weightedDestinationGoodUs = &v1.Destination{DestinationType: &v1.Destination_Upstream{Upstream: &coreV1.ResourceRef{Name: "some-us", Namespace: "bar"}}}
		})
		It("Errors with a WeightedDestination where the upstream namespace doesn't match the secret", func() {
			routeParamsWithSecret := plugins.RouteActionParams{
				RouteParams: plugins.RouteParams{VirtualHostParams: plugins.VirtualHostParams{Params: paramsWithSecret}},
			}
			out := &envoy_config_route_v3.WeightedCluster_ClusterWeight{}
			in := &v1.WeightedDestination{
				Options:     &v1.WeightedDestinationOptions{HeaderManipulation: testHeaderManipWithSecrets},
				Destination: weightedDestinationBadUs,
			}

			err := p.ProcessWeightedDestination(routeParamsWithSecret, in, out)
			Expect(err).To(MatchError("list did not find secret bar.foo"))
		})
		It("Does not error with a good WeightedDestination", func() {
			routeParamsWithSecret := plugins.RouteActionParams{
				RouteParams: plugins.RouteParams{VirtualHostParams: plugins.VirtualHostParams{Params: paramsWithSecret}},
			}
			out := &envoy_config_route_v3.WeightedCluster_ClusterWeight{}
			in := &v1.WeightedDestination{
				Options:     &v1.WeightedDestinationOptions{HeaderManipulation: testHeaderManipWithSecrets},
				Destination: weightedDestinationGoodUs,
			}

			err := p.ProcessWeightedDestination(routeParamsWithSecret, in, out)
			Expect(err).NotTo(HaveOccurred())
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
			singleRouteToBadUpstream.Options = &v1.RouteOptions{HeaderManipulation: testHeaderManipWithSecrets}
			err := p.ProcessRoute(routeParamsWithSecret, singleRouteToBadUpstream, out)
			Expect(err).To(MatchError("list did not find secret bar.foo"))
		})
		It("Does not error when Route destination matches secret", func() {
			routeParamsWithSecret := plugins.RouteParams{VirtualHostParams: plugins.VirtualHostParams{Params: paramsWithSecret}}
			out := &envoy_config_route_v3.Route{}
			singleRouteToGoodUpstream.Options = &v1.RouteOptions{HeaderManipulation: testHeaderManipWithSecrets}
			err := p.ProcessRoute(routeParamsWithSecret, singleRouteToGoodUpstream, out)
			Expect(err).NotTo(HaveOccurred())
		})
		It("Errors with a malformed value for setting", func() {
			os.Setenv(api_conversion.MatchingNamespaceEnv, "tr")
			routeParamsWithSecret := plugins.RouteParams{VirtualHostParams: plugins.VirtualHostParams{Params: paramsWithSecret}}
			out := &envoy_config_route_v3.Route{}
			singleRouteToGoodUpstream.Options = &v1.RouteOptions{HeaderManipulation: testHeaderManipWithSecrets}
			err := p.ProcessRoute(routeParamsWithSecret, singleRouteToGoodUpstream, out)
			Expect(err).Should(MatchError("strconv.ParseBool: parsing \"tr\": invalid syntax"))
		})
		AfterEach(func() {
			os.Clearenv()
			p = NewPlugin()
		})
	})

	Context("ProcessHcmNetworkFilter", func() {
		var (
			plugin         *plugin
			pluginParams   plugins.Params
			parentListener *v1.Listener
			listener       *v1.HttpListener
		)

		BeforeEach(func() {
			plugin = NewPlugin()
			pluginParams = plugins.Params{}
			parentListener = &v1.Listener{}
			listener = &v1.HttpListener{}
		})

		DescribeTable("Early header manipulation transforms",
			func(params plugins.Params, inEhm *headers.EarlyHeaderManipulation,
				expectedOutMutations []*envoy_config_mutation_rules_v3.HeaderMutation, outError error) {
				listener.Options = &v1.HttpListenerOptions{
					HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
						EarlyHeaderManipulation: inEhm,
					},
				}

				out := &envoyhttp.HttpConnectionManager{}
				err := plugin.ProcessHcmNetworkFilter(params, parentListener, listener, out)
				if outError == nil {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal(outError.Error()))
				}

				if expectedOutMutations == nil {
					Expect(out.EarlyHeaderMutationExtensions).To(BeNil())
					return
				}

				Expect(out.EarlyHeaderMutationExtensions).To(HaveLen(1))

				outEhm := &envoy_ehm_header_mutation_v3.HeaderMutation{}
				out.EarlyHeaderMutationExtensions[0].TypedConfig.UnmarshalTo(outEhm)
				Expect(outEhm.Mutations).To(HaveLen(len(expectedOutMutations)))

				for i, expectedOutMutation := range expectedOutMutations {
					outMutation := outEhm.Mutations[i]
					Expect(outMutation.GetAction()).To(Equal(expectedOutMutation.GetAction()))
				}
			},
			Entry("should have nil mutations when emh is nil", pluginParams, nil, nil, nil),
			Entry("should have no mutations when emh is empty",
				pluginParams,
				&headers.EarlyHeaderManipulation{},
				[]*envoy_config_mutation_rules_v3.HeaderMutation{},
				nil,
			),
			Entry("should have remove action when emh has remove",
				pluginParams,
				&headers.EarlyHeaderManipulation{
					HeadersToRemove: testHeaderManip.RequestHeadersToRemove,
				},
				[]*envoy_config_mutation_rules_v3.HeaderMutation{
					{
						Action: &envoy_config_mutation_rules_v3.HeaderMutation_Remove{
							Remove: "a",
						},
					},
				},
				nil,
			),
			Entry("should have append+add append action when emh has add w/ append",
				pluginParams,
				&headers.EarlyHeaderManipulation{
					HeadersToAdd: testHeaderManip.RequestHeadersToAdd,
				},
				[]*envoy_config_mutation_rules_v3.HeaderMutation{
					{
						Action: &envoy_config_mutation_rules_v3.HeaderMutation_Append{
							Append: &envoy_config_core_v3.HeaderValueOption{
								Header: &envoy_config_core_v3.HeaderValue{
									Key:   "foo",
									Value: "bar",
								},
								AppendAction: envoy_config_core_v3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
							},
						},
					},
				},
				nil,
			),
			Entry("should have overwrite+add append action when emh has add w/ overwrite",
				pluginParams,
				&headers.EarlyHeaderManipulation{
					HeadersToAdd: testHeaderManipOverwrite.RequestHeadersToAdd,
				},
				[]*envoy_config_mutation_rules_v3.HeaderMutation{
					{
						Action: &envoy_config_mutation_rules_v3.HeaderMutation_Append{
							Append: &envoy_config_core_v3.HeaderValueOption{
								Header: &envoy_config_core_v3.HeaderValue{
									Key:   "foo",
									Value: "bar",
								},
								AppendAction: envoy_config_core_v3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
							},
						},
					},
				},
				nil,
			),
			Entry("should have append action with missing secret",
				pluginParams,
				&headers.EarlyHeaderManipulation{
					HeadersToAdd: testHeaderManipWithSecrets.RequestHeadersToAdd,
				},
				nil,
				errors.New("list did not find secret bar.foo"),
			),
			Entry("should have append action with missing secret",
				paramsWithSecret,
				&headers.EarlyHeaderManipulation{
					HeadersToAdd: testHeaderManipWithSecrets.RequestHeadersToAdd,
				},
				[]*envoy_config_mutation_rules_v3.HeaderMutation{
					{
						Action: &envoy_config_mutation_rules_v3.HeaderMutation_Append{
							Append: &envoy_config_core_v3.HeaderValueOption{
								Header: &envoy_config_core_v3.HeaderValue{
									Key:   "Authorization",
									Value: "basic dXNlcjpwYXNzd29yZA==",
								},
								AppendAction: envoy_config_core_v3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
							},
						},
					},
				},
				nil,
			),
		)
	})
})

var paramsWithSecret = plugins.Params{
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
	RequestHeadersToAdd:     []*envoy_config_core_v3.HeaderValueOption{{Header: &envoy_config_core_v3.HeaderValue{Key: "foo", Value: "bar"}, AppendAction: envoy_config_core_v3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD}},
	RequestHeadersToRemove:  []string{"a"},
	ResponseHeadersToAdd:    []*envoy_config_core_v3.HeaderValueOption{{Header: &envoy_config_core_v3.HeaderValue{Key: "foo", Value: "bar"}, AppendAction: envoy_config_core_v3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD}},
	ResponseHeadersToRemove: []string{"b"},
}

var testHeaderManipOverwrite = &headers.HeaderManipulation{
	RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{{HeaderOption: &envoycore_sk.HeaderValueOption_Header{Header: &envoycore_sk.HeaderValue{Key: "foo", Value: "bar"}},
		Append: &wrappers.BoolValue{Value: false}}},
	RequestHeadersToRemove:  []string{"a"},
	ResponseHeadersToAdd:    []*headers.HeaderValueOption{{Header: &headers.HeaderValue{Key: "foo", Value: "bar"}, Append: &wrappers.BoolValue{Value: false}}},
	ResponseHeadersToRemove: []string{"b"},
}

var expectedHeadersOverwrite = envoyHeaderManipulation{
	RequestHeadersToAdd:     []*envoy_config_core_v3.HeaderValueOption{{Header: &envoy_config_core_v3.HeaderValue{Key: "foo", Value: "bar"}, AppendAction: envoy_config_core_v3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD}},
	RequestHeadersToRemove:  []string{"a"},
	ResponseHeadersToAdd:    []*envoy_config_core_v3.HeaderValueOption{{Header: &envoy_config_core_v3.HeaderValue{Key: "foo", Value: "bar"}, AppendAction: envoy_config_core_v3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD}},
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
	RequestHeadersToAdd:     []*envoy_config_core_v3.HeaderValueOption{{Header: &envoy_config_core_v3.HeaderValue{Key: "Authorization", Value: "basic dXNlcjpwYXNzd29yZA=="}, AppendAction: envoy_config_core_v3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD}},
	RequestHeadersToRemove:  []string{"a"},
	ResponseHeadersToAdd:    []*envoy_config_core_v3.HeaderValueOption{{Header: &envoy_config_core_v3.HeaderValue{Key: "foo", Value: "bar"}, AppendAction: envoy_config_core_v3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD}},
	ResponseHeadersToRemove: []string{"b"},
}
