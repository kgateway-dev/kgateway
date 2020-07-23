package transformation_test

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/route/v3"
	matcherv3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	"google.golang.org/protobuf/types/known/structpb"

	envoytransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
)

var _ = Describe("Plugin", func() {
	var (
		p               *Plugin
		expected        *structpb.Struct
		outputTransform *envoytransformation.RouteTransformations
	)

	Context("deprecated transformations", func() {
		var (
			inputTransform *transformation.Transformations
		)
		BeforeEach(func() {
			p = NewPlugin()
			inputTransform = &transformation.Transformations{
				ClearRouteCache: true,
			}
			outputTransform = &envoytransformation.RouteTransformations{
				// deprecated config gets old and new config
				ClearRouteCache: true,
				Transformations: []*envoytransformation.RouteTransformations_RouteTransformation{
					{
						Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
							RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{ClearRouteCache: true},
						},
					},
				},
			}
			configStruct, err := conversion.MessageToStruct(outputTransform)
			Expect(err).NotTo(HaveOccurred())

			expected = configStruct
		})

		It("sets transformation config for weighted destinations", func() {
			out := &envoyroute.WeightedCluster_ClusterWeight{}
			err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
				Options: &v1.WeightedDestinationOptions{
					Transformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.PerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("sets transformation config for virtual hosts", func() {
			out := &envoyroute.VirtualHost{}
			err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					Transformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.PerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("sets transformation config for routes", func() {
			out := &envoyroute.Route{}
			err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
				Options: &v1.RouteOptions{
					Transformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.PerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("sets only one filter when no early filters exist", func() {
			filters, err := p.HttpFilters(plugins.Params{}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(filters)).To(Equal(1))
			value := filters[0].HttpFilter.GetConfig()
			Expect(value).To(BeNil())
		})
	})

	Context("staged transformations", func() {
		var (
			inputTransform         *transformation.TransformationStages
			earlyStageFilterConfig *structpb.Struct
		)
		BeforeEach(func() {
			p = NewPlugin()
			var err error
			earlyStageFilterConfig, err = conversion.MessageToStruct(&envoytransformation.FilterTransformations{
				Stage: EarlyStageNumber,
			})
			Expect(err).NotTo(HaveOccurred())
			// construct transformation with all the options, to make sure translation is correct
			earlyRequestTransform := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_TransformationTemplate{
					TransformationTemplate: &envoytransformation.TransformationTemplate{
						AdvancedTemplates: true,
						BodyTransformation: &envoytransformation.TransformationTemplate_Body{
							Body: &envoytransformation.InjaTemplate{Text: "1"},
						},
					},
				},
			}
			earlyResponseTransform := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_TransformationTemplate{
					TransformationTemplate: &envoytransformation.TransformationTemplate{
						AdvancedTemplates: true,
						BodyTransformation: &envoytransformation.TransformationTemplate_Body{
							Body: &envoytransformation.InjaTemplate{Text: "2"},
						},
					},
				},
			}
			requestTransform := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_TransformationTemplate{
					TransformationTemplate: &envoytransformation.TransformationTemplate{
						AdvancedTemplates: true,
						BodyTransformation: &envoytransformation.TransformationTemplate_Body{
							Body: &envoytransformation.InjaTemplate{Text: "11"},
						},
					},
				},
			}
			responseTransform := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_TransformationTemplate{
					TransformationTemplate: &envoytransformation.TransformationTemplate{
						AdvancedTemplates: true,
						BodyTransformation: &envoytransformation.TransformationTemplate_Body{
							Body: &envoytransformation.InjaTemplate{Text: "12"},
						},
					},
				},
			}
			inputTransform = &transformation.TransformationStages{
				Early: &transformation.RequestResponseTransformations{
					ResponseTransforms: []*transformation.ResponseMatch{
						{
							Matchers: []*matchers.HeaderMatcher{
								{
									Name:  "foo",
									Value: "bar",
								},
							},
							ResponseCodeDetails:    "abcd",
							ResponseTransformation: earlyResponseTransform,
						},
					},
					RequestTransforms: []*transformation.RequestMatch{
						{
							Matcher:                &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo"}},
							ClearRouteCache:        true,
							RequestTransformation:  earlyRequestTransform,
							ResponseTransformation: earlyResponseTransform,
						},
					},
				},
				Regular: &transformation.RequestResponseTransformations{
					ResponseTransforms: []*transformation.ResponseMatch{
						{
							Matchers: []*matchers.HeaderMatcher{
								{
									Name:  "foo",
									Value: "bar",
								},
							},
							ResponseCodeDetails:    "abcd",
							ResponseTransformation: earlyResponseTransform,
						},
					},
					RequestTransforms: []*transformation.RequestMatch{
						{
							Matcher:                &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo2"}},
							ClearRouteCache:        true,
							RequestTransformation:  requestTransform,
							ResponseTransformation: responseTransform,
						},
					},
				},
			}
			outputTransform = &envoytransformation.RouteTransformations{
				// new config should not get deprecated config
				Transformations: []*envoytransformation.RouteTransformations_RouteTransformation{
					{
						Stage: EarlyStageNumber,
						Match: &envoytransformation.RouteTransformations_RouteTransformation_ResponseMatch_{
							ResponseMatch: &envoytransformation.RouteTransformations_RouteTransformation_ResponseMatch{
								Match: &envoytransformation.ResponseMatcher{
									Headers: []*v3.HeaderMatcher{
										{
											Name:                 "foo",
											HeaderMatchSpecifier: &v3.HeaderMatcher_ExactMatch{ExactMatch: "bar"},
										},
									},
									ResponseCodeDetails: &matcherv3.StringMatcher{
										MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "abcd"},
									},
								},
								ResponseTransformation: earlyResponseTransform,
							},
						},
					},
					{
						Stage: EarlyStageNumber,
						Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
							RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
								Match:                  &v3.RouteMatch{PathSpecifier: &v3.RouteMatch_Prefix{Prefix: "/foo"}},
								ClearRouteCache:        true,
								RequestTransformation:  earlyRequestTransform,
								ResponseTransformation: earlyResponseTransform,
							},
						},
					},
					{
						Match: &envoytransformation.RouteTransformations_RouteTransformation_ResponseMatch_{
							ResponseMatch: &envoytransformation.RouteTransformations_RouteTransformation_ResponseMatch{
								Match: &envoytransformation.ResponseMatcher{
									Headers: []*v3.HeaderMatcher{
										{
											Name:                 "foo",
											HeaderMatchSpecifier: &v3.HeaderMatcher_ExactMatch{ExactMatch: "bar"},
										},
									},
									ResponseCodeDetails: &matcherv3.StringMatcher{
										MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "abcd"},
									},
								},
								ResponseTransformation: earlyResponseTransform,
							},
						},
					},
					{
						Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
							RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
								Match:                  &v3.RouteMatch{PathSpecifier: &v3.RouteMatch_Prefix{Prefix: "/foo2"}},
								ClearRouteCache:        true,
								RequestTransformation:  requestTransform,
								ResponseTransformation: responseTransform,
							},
						},
					},
				},
			}
			configStruct, err := conversion.MessageToStruct(outputTransform)
			Expect(err).NotTo(HaveOccurred())

			expected = configStruct
		})
		It("sets transformation config for vhosts", func() {
			out := &envoyroute.VirtualHost{}
			err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					StagedTransformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.PerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("sets transformation config for routes", func() {
			out := &envoyroute.Route{}
			err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
				Options: &v1.RouteOptions{
					StagedTransformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.PerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("sets transformation config for weighted dest", func() {
			out := &envoyroute.WeightedCluster_ClusterWeight{}
			err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
				Options: &v1.WeightedDestinationOptions{
					StagedTransformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.PerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("should add both filter to the chain when early transformations exist", func() {
			out := &envoyroute.Route{}
			err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
				Options: &v1.RouteOptions{
					StagedTransformations: inputTransform,
				},
			}, out)
			filters, err := p.HttpFilters(plugins.Params{}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(filters)).To(Equal(2))
			value := filters[0].HttpFilter.GetConfig()
			Expect(value).To(Equal(earlyStageFilterConfig))
			// second filter should have no stage, and thus empty config
			value = filters[1].HttpFilter.GetConfig()
			Expect(value).To(BeNil())
		})
	})

})
