package csrf_test

import (
	envoycsrf "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	envoyhcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"

	api_type_matcher_v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v31 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/csrf/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/csrf"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

)

var _ = Describe("Plugin", func() {
	It("copies the csrf config from the listener to the filter with AdditionalOrigins set", func() {
		additionalOrigins := []*api_type_matcher_v3.StringMatcher {
			&api_type_matcher_v3.StringMatcher{
				MatchPattern: &api_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		envoy_additionalOrigins := []*envoy_type_matcher_v3.StringMatcher {
			&envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
					Exact: "test",
				},
				IgnoreCase: true,
			},
		}

		filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Csrf: &v31.CsrfPolicy{
					FilterEnabled: nil,
					ShadowEnabled: nil,
					AdditionalOrigins: additionalOrigins,
				},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(Equal([]plugins.StagedHttpFilter{
			plugins.StagedHttpFilter{
				HttpFilter: &envoyhcm.HttpFilter{
					Name: "envoy.filters.http.csrf",
					ConfigType: &envoyhcm.HttpFilter_TypedConfig{
						TypedConfig: utils.MustMessageToAny(&envoycsrf.CsrfPolicy{
							FilterEnabled: nil,
							ShadowEnabled: nil,
							AdditionalOrigins: envoy_additionalOrigins,
						}),
					},
				},
				Stage: plugins.FilterStage{
					RelativeTo: 8,
					Weight:     0,
				},
			},
		}))
	})

})
