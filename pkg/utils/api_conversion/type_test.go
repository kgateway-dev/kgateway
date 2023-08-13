package api_conversion_test

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	. "github.com/solo-io/gloo/pkg/utils/api_conversion"
	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
)

var _ = Describe("Type conversion", func() {
	Context("ToEnvoyHeaderValueOptionList", func() {
		It("should convert allowed headers", func() {
			allowedHeaders := []*envoycore_sk.HeaderValueOption{
				{
					HeaderOption: &envoycore_sk.HeaderValueOption_Header{
						Header: &envoycore_sk.HeaderValue{
							Key:   "allowed",
							Value: "header",
						},
					},
					Append: &wrappers.BoolValue{
						Value: true,
					},
				},
			}

			expectedHeaders := []*envoy_config_core_v3.HeaderValueOption{
				{
					Header: &envoy_config_core_v3.HeaderValue{
						Key:   "allowed",
						Value: "header",
					},
					Append: &wrappers.BoolValue{
						Value: true,
					},
				},
			}
			headers, err := ToEnvoyHeaderValueOptionList(allowedHeaders, nil, HeaderSecretOptions{})
			Expect(err).To(BeNil())
			Expect(headers).To(Equal(expectedHeaders))
		})

		DescribeTable("should error out forbidden headers", func(key string) {
			allowedHeaders := []*envoycore_sk.HeaderValueOption{
				{
					HeaderOption: &envoycore_sk.HeaderValueOption_Header{
						Header: &envoycore_sk.HeaderValue{
							Key:   key,
							Value: "value",
						},
					},
					Append: &wrappers.BoolValue{
						Value: true,
					},
				},
			}

			_, err := ToEnvoyHeaderValueOptionList(allowedHeaders, nil, HeaderSecretOptions{})
			Expect(err).To(MatchError(ContainSubstring(": -prefixed or host headers may not be modified")))
		},
			Entry("host header", "host"),
			Entry(": prefixed header header", ":path"))
	})
})
