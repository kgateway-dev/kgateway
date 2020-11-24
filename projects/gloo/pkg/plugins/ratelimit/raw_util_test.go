package ratelimit_test

import (
	"fmt"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/ratelimit"
	gloorl "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
)

var _ = Describe("RawUtil", func() {

	Context("should convert protos to the same thing till we properly vendor them", func() {
		It("should convert source cluster", func() {
			inactions := []*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_SourceCluster_{
					SourceCluster: &gloorl.Action_SourceCluster{},
				},
			}}
			ExpectActionsSame(inactions)
		})
		It("should convert dest cluster", func() {
			inactions := []*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_DestinationCluster_{
					DestinationCluster: &gloorl.Action_DestinationCluster{},
				},
			}}
			ExpectActionsSame(inactions)
		})
		It("should convert generic key", func() {
			inactions := []*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_GenericKey_{
					GenericKey: &gloorl.Action_GenericKey{
						DescriptorValue: "somevalue",
					},
				},
			}}
			ExpectActionsSame(inactions)
		})
		It("should convert remote address", func() {
			inactions := []*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_RemoteAddress_{
					RemoteAddress: &gloorl.Action_RemoteAddress{},
				},
			}}
			ExpectActionsSame(inactions)
		})
		It("should convert request headers", func() {
			inactions := []*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_RequestHeaders_{
					RequestHeaders: &gloorl.Action_RequestHeaders{
						DescriptorKey: "key",
						HeaderName:    "name",
					},
				},
			}}
			ExpectActionsSame(inactions)
		})
		FIt("should convert headermatch", func() {
			m := []*gloorl.Action_HeaderValueMatch_HeaderMatcher{{
				HeaderMatchSpecifier: &gloorl.Action_HeaderValueMatch_HeaderMatcher_ExactMatch{
					ExactMatch: "e",
				},
				Name: "test",
			}, {
				HeaderMatchSpecifier: &gloorl.Action_HeaderValueMatch_HeaderMatcher_RegexMatch{
					RegexMatch: "r",
				},
				Name:        "test",
				InvertMatch: true,
			}, {
				HeaderMatchSpecifier: &gloorl.Action_HeaderValueMatch_HeaderMatcher_PresentMatch{
					PresentMatch: true,
				},
				Name:        "tests",
				InvertMatch: true,
			}, {
				HeaderMatchSpecifier: &gloorl.Action_HeaderValueMatch_HeaderMatcher_PrefixMatch{
					PrefixMatch: "r",
				},
				Name: "test",
			}, {
				HeaderMatchSpecifier: &gloorl.Action_HeaderValueMatch_HeaderMatcher_SuffixMatch{
					SuffixMatch: "r",
				},
				Name: "test",
			}, {
				HeaderMatchSpecifier: &gloorl.Action_HeaderValueMatch_HeaderMatcher_RangeMatch{
					RangeMatch: &gloorl.Action_HeaderValueMatch_HeaderMatcher_Int64Range{
						Start: 123,
						End:   134,
					},
				},
				Name: "test",
			},
			}

			inactions := []*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_HeaderValueMatch_{
					HeaderValueMatch: &gloorl.Action_HeaderValueMatch{
						DescriptorValue: "somevalue",
						ExpectMatch:     &types.BoolValue{Value: true},
						Headers:         m,
					},
				},
			}, {
				ActionSpecifier: &gloorl.Action_HeaderValueMatch_{
					HeaderValueMatch: &gloorl.Action_HeaderValueMatch{
						DescriptorValue: "someothervalue",
						ExpectMatch:     &types.BoolValue{Value: false},
						Headers:         m,
					},
				},
			},
			}
			ExpectActionsSame(inactions)
		})

	})

})

func ExpectActionsSame(actions []*gloorl.Action) {
	out := ConvertActions(nil, actions)

	ExpectWithOffset(1, len(actions)).To(Equal(len(out)))
	for i := range actions {

		jase := jsonpb.Marshaler{}
		ins, _ := jase.MarshalToString(actions[i])
		outs, _ := jase.MarshalToString(out[i])
		fmt.Fprintf(GinkgoWriter, "Compare \n%s\n\n%s", ins, outs)
		remarshalled := new(envoy_config_route_v3.RateLimit_Action)
		err := jsonpb.UnmarshalString(ins, remarshalled)

		// regex api is different. fix that.
		if headers := remarshalled.GetHeaderValueMatch().GetHeaders(); headers != nil {
			for _, h := range headers {
				if regex := h.GetHiddenEnvoyDeprecatedRegexMatch(); regex != "" {
					h.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_HiddenEnvoyDeprecatedRegexMatch{
						HiddenEnvoyDeprecatedRegexMatch: regex,
					}
				}
			}
		}

		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ExpectWithOffset(1, remarshalled).To(Equal(out[i]))
	}

}
