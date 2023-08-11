package ratelimit_test

import (
	"fmt"

	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	golangjsonpb "github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes/wrappers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/ratelimit"
	gloorl "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
)

var _ = Describe("RawUtil", func() {

	var (
		hm = []*gloorl.Action_HeaderValueMatch_HeaderMatcher{
			{
				HeaderMatchSpecifier: &gloorl.Action_HeaderValueMatch_HeaderMatcher_ExactMatch{
					ExactMatch: "e",
				},
				Name: "test",
			},
			{
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
	)

	// note: this is no longer a straight conversion, see the other context below for other tests
	DescribeTable(
		"should convert protos to the same thing till we properly vendor them",
		func(actions []*gloorl.Action) {
			out, err := ConvertActions(nil, actions)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(actions)).To(Equal(len(out)))
			for i := range actions {
				golangjson := golangjsonpb.Marshaler{}

				ins, _ := golangjson.MarshalToString(actions[i])
				outs, _ := golangjson.MarshalToString(out[i])
				fmt.Fprintf(GinkgoWriter, "Compare \n%s\n\n%s", ins, outs)
				remarshalled := new(envoy_config_route_v3.RateLimit_Action)
				err := golangjsonpb.UnmarshalString(ins, remarshalled)
				Expect(err).NotTo(HaveOccurred())
				Expect(remarshalled).To(Equal(out[i]))
			}
		},
		Entry("should convert source cluster",
			[]*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_SourceCluster_{
					SourceCluster: &gloorl.Action_SourceCluster{},
				},
			}},
		),
		Entry("should convert dest cluster",
			[]*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_DestinationCluster_{
					DestinationCluster: &gloorl.Action_DestinationCluster{},
				},
			}},
		),
		Entry("should convert generic key",
			[]*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_GenericKey_{
					GenericKey: &gloorl.Action_GenericKey{
						DescriptorValue: "somevalue",
					},
				},
			}},
		),
		Entry("should convert remote address",
			[]*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_RemoteAddress_{
					RemoteAddress: &gloorl.Action_RemoteAddress{},
				},
			}},
		),
		Entry("should convert request headers",
			[]*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_RequestHeaders_{
					RequestHeaders: &gloorl.Action_RequestHeaders{
						DescriptorKey: "key",
						HeaderName:    "name",
					},
				},
			}},
		),
		Entry("should convert headermatch",
			[]*gloorl.Action{
				{
					ActionSpecifier: &gloorl.Action_HeaderValueMatch_{
						HeaderValueMatch: &gloorl.Action_HeaderValueMatch{
							DescriptorValue: "somevalue",
							ExpectMatch:     &wrappers.BoolValue{Value: true},
							Headers:         hm,
						},
					},
				}, {
					ActionSpecifier: &gloorl.Action_HeaderValueMatch_{
						HeaderValueMatch: &gloorl.Action_HeaderValueMatch{
							DescriptorValue: "someothervalue",
							ExpectMatch:     &wrappers.BoolValue{Value: false},
							Headers:         hm,
						},
					},
				},
			},
		),
		Entry("should convert metadata",
			[]*gloorl.Action{
				{
					ActionSpecifier: &gloorl.Action_Metadata{
						Metadata: &gloorl.Action_MetaData{
							DescriptorKey: "some-key",
							MetadataKey: &gloorl.Action_MetaData_MetadataKey{
								Key: "io.solo.some.filter",
								Path: []*gloorl.Action_MetaData_MetadataKey_PathSegment{
									{
										Segment: &gloorl.Action_MetaData_MetadataKey_PathSegment_Key{
											Key: "foo",
										},
									},
								},
							},
							DefaultValue: "nothing",
							Source:       gloorl.Action_MetaData_ROUTE_ENTRY,
						},
					},
				},
				{
					ActionSpecifier: &gloorl.Action_Metadata{
						Metadata: &gloorl.Action_MetaData{
							DescriptorKey: "some-other-key",
							MetadataKey: &gloorl.Action_MetaData_MetadataKey{
								Key: "io.solo.some.other.filter",
								// no path here
							},
						},
					},
				},
			},
		),
	)
	DescribeTable("Errors on missing required fields", func(actions []*gloorl.Action) {
		_, err := ConvertActions(nil, actions)
		Expect(err).To(MatchError(ContainSubstring("Missing required field in ratelimit action")))
	},
		Entry("Missing descriptorValue in genericKey",
			[]*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_GenericKey_{
					GenericKey: &gloorl.Action_GenericKey{},
				},
			}},
		),
		Entry("Missing headername in request headers",
			[]*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_RequestHeaders_{
					RequestHeaders: &gloorl.Action_RequestHeaders{
						DescriptorKey: "key",
					},
				},
			}},
		),
		Entry("Missing descriptor key in request headers",
			[]*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_RequestHeaders_{
					RequestHeaders: &gloorl.Action_RequestHeaders{
						HeaderName: "header",
					},
				},
			}},
		),
		Entry("Missing descriptorValue in headerValueMatch",
			[]*gloorl.Action{
				{
					ActionSpecifier: &gloorl.Action_HeaderValueMatch_{
						HeaderValueMatch: &gloorl.Action_HeaderValueMatch{
							ExpectMatch: &wrappers.BoolValue{Value: true},
							Headers:     hm,
						},
					},
				}},
		),
		Entry("Missing DescriptorKey in ActionMetadata",
			[]*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_Metadata{
					Metadata: &gloorl.Action_MetaData{
						MetadataKey: &gloorl.Action_MetaData_MetadataKey{
							Key: "io.solo.some.filter",
							Path: []*gloorl.Action_MetaData_MetadataKey_PathSegment{
								{
									Segment: &gloorl.Action_MetaData_MetadataKey_PathSegment_Key{
										Key: "foo",
									},
								},
							},
						},
						DefaultValue: "nothing",
						Source:       gloorl.Action_MetaData_ROUTE_ENTRY,
					},
				},
			}},
		),
		Entry("Missing metadataKey_key in ActionMetadata",
			[]*gloorl.Action{{
				ActionSpecifier: &gloorl.Action_Metadata{
					Metadata: &gloorl.Action_MetaData{
						DescriptorKey: "some-key",
						MetadataKey: &gloorl.Action_MetaData_MetadataKey{
							Path: []*gloorl.Action_MetaData_MetadataKey_PathSegment{
								{
									Segment: &gloorl.Action_MetaData_MetadataKey_PathSegment_Key{
										Key: "foo",
									},
								},
							},
						},
						DefaultValue: "nothing",
						Source:       gloorl.Action_MetaData_ROUTE_ENTRY,
					},
				},
			}}),
	)
	// Needs to be separate because the yaml is no longer compatible
	Context("special cases - not a straight conversion", func() {

		It("works with regex", func() {
			actions := []*gloorl.Action{
				{
					ActionSpecifier: &gloorl.Action_HeaderValueMatch_{
						HeaderValueMatch: &gloorl.Action_HeaderValueMatch{
							DescriptorValue: "someothervalue",
							ExpectMatch:     &wrappers.BoolValue{Value: false},
							Headers: []*gloorl.Action_HeaderValueMatch_HeaderMatcher{
								{
									HeaderMatchSpecifier: &gloorl.Action_HeaderValueMatch_HeaderMatcher_RegexMatch{
										RegexMatch: "hello",
									},
									Name: "test",
								},
							},
						},
					},
				},
			}

			out, err := ConvertActions(nil, actions)
			Expect(err).NotTo(HaveOccurred())
			expected := []*envoy_config_route_v3.RateLimit_Action{
				{
					ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_HeaderValueMatch_{
						HeaderValueMatch: &envoy_config_route_v3.RateLimit_Action_HeaderValueMatch{
							DescriptorValue: "someothervalue",
							ExpectMatch: &wrappers.BoolValue{
								Value: false,
							},
							Headers: []*envoy_config_route_v3.HeaderMatcher{
								{
									Name: "test",
									HeaderMatchSpecifier: &envoy_config_route_v3.HeaderMatcher_SafeRegexMatch{
										SafeRegexMatch: &envoy_type_matcher_v3.RegexMatcher{
											EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
												GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{
													MaxProgramSize: nil,
												},
											},
											Regex: "hello",
										},
									},
									InvertMatch: false,
								},
							},
						},
					},
				},
			}

			Expect(out).To(Equal(expected))

		})

		It("works with set actions and request headers", func() {
			actions := []*gloorl.Action{
				{
					// our special generic key that signals to treat the rest of the actions as a set
					ActionSpecifier: &gloorl.Action_GenericKey_{
						GenericKey: &gloorl.Action_GenericKey{DescriptorValue: SetDescriptorValue},
					},
				},
				{
					ActionSpecifier: &gloorl.Action_RequestHeaders_{
						RequestHeaders: &gloorl.Action_RequestHeaders{
							HeaderName:    "x-foo",
							DescriptorKey: "foo",
						},
					},
				},
				{
					ActionSpecifier: &gloorl.Action_RequestHeaders_{
						RequestHeaders: &gloorl.Action_RequestHeaders{
							HeaderName:    "x-bar",
							DescriptorKey: "bar",
						},
					},
				},
			}

			out, err := ConvertActions(nil, actions)
			Expect(err).NotTo(HaveOccurred())
			expected := []*envoy_config_route_v3.RateLimit_Action{
				{
					ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_GenericKey_{
						GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{DescriptorValue: SetDescriptorValue},
					},
				},
				{
					ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_RequestHeaders_{
						RequestHeaders: &envoy_config_route_v3.RateLimit_Action_RequestHeaders{
							HeaderName:    "x-foo",
							DescriptorKey: "foo",
							SkipIfAbsent:  true, // important, or else rate-limit server won't get requests if some headers are missing from a request
						},
					},
				},
				{
					ActionSpecifier: &envoy_config_route_v3.RateLimit_Action_RequestHeaders_{
						RequestHeaders: &envoy_config_route_v3.RateLimit_Action_RequestHeaders{
							HeaderName:    "x-bar",
							DescriptorKey: "bar",
							SkipIfAbsent:  true, // important, or else rate-limit server won't get requests if some headers are missing from a request
						},
					},
				},
			}

			Expect(out).To(Equal(expected))

		})

	})

})
