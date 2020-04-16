package ratelimit_test

import (
	"fmt"

	envoyvhostratelimit "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"
	regexutils "github.com/solo-io/gloo/pkg/utils/regexutils"
	gloorl "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/ratelimit"
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
		It("should convert headermatch", func() {
			m := []*gloorl.HeaderMatcher{{
				HeaderMatchSpecifier: &gloorl.HeaderMatcher_ExactMatch{
					ExactMatch: "e",
				},
				Name: "test",
			}, {
				HeaderMatchSpecifier: &gloorl.HeaderMatcher_RegexMatch{
					RegexMatch: "r",
				},
				Name:        "test",
				InvertMatch: true,
			}, {
				HeaderMatchSpecifier: &gloorl.HeaderMatcher_PresentMatch{
					PresentMatch: true,
				},
				Name:        "tests",
				InvertMatch: true,
			}, {
				HeaderMatchSpecifier: &gloorl.HeaderMatcher_PrefixMatch{
					PrefixMatch: "r",
				},
				Name: "test",
			}, {
				HeaderMatchSpecifier: &gloorl.HeaderMatcher_SuffixMatch{
					SuffixMatch: "r",
				},
				Name: "test",
			}, {
				HeaderMatchSpecifier: &gloorl.HeaderMatcher_RangeMatch{
					RangeMatch: &gloorl.Int64Range{
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
	out := ConvertActions(actions)

	ExpectWithOffset(1, len(actions)).To(Equal(len(out)))
	for i := range actions {

		jase := jsonpb.Marshaler{}
		ins, _ := jase.MarshalToString(actions[i])
		outs, _ := jase.MarshalToString(out[i])
		fmt.Fprintf(GinkgoWriter, "Compare \n%s\n\n%s", ins, outs)
		remarshalled := new(envoyvhostratelimit.RateLimit_Action)
		err := jsonpb.UnmarshalString(ins, remarshalled)

		// regex api is different. fix that.
		if headers := remarshalled.GetHeaderValueMatch().GetHeaders(); headers != nil {
			for _, h := range headers {
				if regex := h.GetRegexMatch(); regex != "" {
					h.HeaderMatchSpecifier = &envoyvhostratelimit.HeaderMatcher_SafeRegexMatch{
						SafeRegexMatch: regexutils.NewRegex(params.Ctx, regex),
					}
				}
			}
		}

		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ExpectWithOffset(1, remarshalled).To(Equal(out[i]))
	}

}
