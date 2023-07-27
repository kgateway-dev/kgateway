package transformation_test

import (
	"context"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/ptypes/any"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/route/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformers/xslt"
	matcherv3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoytransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	skMatchers "github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("Plugin", func() {
	var (
		ctx             context.Context
		cancel          context.CancelFunc
		p               plugins.Plugin
		expected        *any.Any
		outputTransform *envoytransformation.RouteTransformations
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
	})

	Context("translate transformations", func() {

		BeforeEach(func() {
			p = NewPlugin()
			p.Init(plugins.InitParams{Ctx: ctx, Settings: &v1.Settings{Gloo: &v1.GlooOptions{RemoveUnusedFilters: &wrapperspb.BoolValue{Value: false}}}})
		})

		It("translates header body transform", func() {
			headerBodyTransformIn := &transformation.HeaderBodyTransform{}
			headerBodyTransform := &envoytransformation.HeaderBodyTransform{}

			input := &transformation.Transformation{
				TransformationType: &transformation.Transformation_HeaderBodyTransform{
					HeaderBodyTransform: headerBodyTransformIn,
				},
			}

			expectedOutput := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_HeaderBodyTransform{
					HeaderBodyTransform: headerBodyTransform,
				},
			}
			output, err := TranslateTransformation(input, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(expectedOutput))
		})

		It("translates transformation template repeatedly", func() {
			transformationTemplateIn := &transformation.TransformationTemplate{
				HeadersToAppend: []*transformation.TransformationTemplate_HeaderToAppend{
					{
						Key: "some-header",
						Value: &transformation.InjaTemplate{
							Text: "some text",
						},
					},
				},
			}
			transformationTemplate := &envoytransformation.TransformationTemplate{
				HeadersToAppend: []*envoytransformation.TransformationTemplate_HeaderToAppend{
					{
						Key: "some-header",
						Value: &envoytransformation.InjaTemplate{
							Text: "some text",
						},
					},
				},
			}

			input := &transformation.Transformation{
				TransformationType: &transformation.Transformation_TransformationTemplate{
					TransformationTemplate: transformationTemplateIn,
				},
			}

			expectedOutput := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_TransformationTemplate{
					TransformationTemplate: transformationTemplate,
				},
			}
			output, err := TranslateTransformation(input, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(expectedOutput))

		})

		It("throws error on unsupported transformation type", func() {
			// Xslt Transformation is enterprise-only
			input := &transformation.Transformation{
				TransformationType: &transformation.Transformation_XsltTransformation{
					XsltTransformation: &xslt.XsltTransformation{
						Xslt: "<xsl:stylesheet>some transform</xsl:stylesheet>",
					},
				},
			}

			output, err := TranslateTransformation(input, nil, nil)
			Expect(output).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(UnknownTransformationType(&transformation.Transformation_XsltTransformation{})))

		})

		Context("LogRequestResponseInfo", func() {

			var (
				inputTransformationStages *transformation.TransformationStages
				expectedOutput            *envoytransformation.RouteTransformations
			)

			type transformationPlugin interface {
				plugins.Plugin
				ConvertTransformation(
					ctx context.Context,
					t *transformation.Transformations,
					stagedTransformations *transformation.TransformationStages,
				) (*envoytransformation.RouteTransformations, error)
			}

			BeforeEach(func() {
				inputTransformationStages = &transformation.TransformationStages{
					Regular: &transformation.RequestResponseTransformations{
						RequestTransforms: []*transformation.RequestMatch{{
							RequestTransformation: &transformation.Transformation{
								TransformationType: &transformation.Transformation_HeaderBodyTransform{
									HeaderBodyTransform: &transformation.HeaderBodyTransform{},
								},
							},
						}},
					},
				}

				expectedOutput = &envoytransformation.RouteTransformations{
					Transformations: []*envoytransformation.RouteTransformations_RouteTransformation{{
						Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
							RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
								RequestTransformation: &envoytransformation.Transformation{
									TransformationType: &envoytransformation.Transformation_HeaderBodyTransform{
										HeaderBodyTransform: &envoytransformation.HeaderBodyTransform{},
									},
								},
							},
						},
					}},
				}
			})

			It("can set log_request_response_info on transformation-stages level", func() {
				inputTransformationStages.LogRequestResponseInfo = &wrapperspb.BoolValue{Value: true}
				expectedOutput.Transformations[0].Match.(*envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_).RequestMatch.RequestTransformation.LogRequestResponseInfo = &wrapperspb.BoolValue{Value: true}

				output, err := p.(transformationPlugin).ConvertTransformation(
					ctx,
					&transformation.Transformations{},
					inputTransformationStages,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal(expectedOutput))
			})

			It("does not set log_request_response_info if transformation-stages-level setting is false", func() {
				inputTransformationStages.LogRequestResponseInfo = &wrapperspb.BoolValue{Value: false}

				output, err := p.(transformationPlugin).ConvertTransformation(
					ctx,
					&transformation.Transformations{},
					inputTransformationStages,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal(expectedOutput))

			})

			It("can set log_request_response_info on transformation level", func() {
				inputTransformationStages.Regular.RequestTransforms[0].RequestTransformation.LogRequestResponseInfo = true
				expectedOutput.Transformations[0].Match.(*envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_).RequestMatch.RequestTransformation.LogRequestResponseInfo = &wrapperspb.BoolValue{Value: true}

				output, err := p.(transformationPlugin).ConvertTransformation(
					ctx,
					&transformation.Transformations{},
					inputTransformationStages,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal(expectedOutput))
			})

			It("does not set log_request_response_info if transformation-level setting is false", func() {
				inputTransformationStages.Regular.RequestTransforms[0].RequestTransformation.LogRequestResponseInfo = false

				output, err := p.(transformationPlugin).ConvertTransformation(
					ctx,
					&transformation.Transformations{},
					inputTransformationStages,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal(expectedOutput))
			})

			It("can override transformation-level log_request_response_info with transformation-stages level", func() {
				inputTransformationStages.Regular.RequestTransforms[0].RequestTransformation.LogRequestResponseInfo = false
				inputTransformationStages.LogRequestResponseInfo = &wrapperspb.BoolValue{Value: true}
				expectedOutput.Transformations[0].Match.(*envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_).RequestMatch.RequestTransformation.LogRequestResponseInfo = &wrapperspb.BoolValue{Value: true}

				output, err := p.(transformationPlugin).ConvertTransformation(
					ctx,
					&transformation.Transformations{},
					inputTransformationStages,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal(expectedOutput))
			})

			It("can enable settings-object-level setting", func() {
				// override plugin created in BeforeEach
				p = NewPlugin()
				// initialize with settings-object-level setting enabled
				p.Init(plugins.InitParams{
					Ctx: ctx,
					Settings: &v1.Settings{
						Gloo: &v1.GlooOptions{
							RemoveUnusedFilters:            &wrapperspb.BoolValue{Value: false},
							TransformationEscapeCharacters: &wrapperspb.BoolValue{Value: true},
						},
					},
				})

				stagedHttpFilters, err := p.(plugins.HttpFilterPlugin).HttpFilters(plugins.Params{}, &v1.HttpListener{})
				Expect(err).NotTo(HaveOccurred())

				Expect(stagedHttpFilters).To(HaveLen(1))
				Expect(stagedHttpFilters[0].HttpFilter.Name).To(Equal("io.solo.transformation"))
				// pretty print the typed config of the filter
				typedConfig := stagedHttpFilters[0].HttpFilter.GetTypedConfig()
				expectedFilter := plugins.MustNewStagedFilter(
					FilterName,
					&envoytransformation.FilterTransformations{
						LogRequestResponseInfo: true,
					},
					plugins.AfterStage(plugins.AuthZStage),
				)

				Expect(typedConfig).To(skMatchers.MatchProto(expectedFilter.HttpFilter.GetTypedConfig()))

				expectedOutput.Transformations[0].Match.(*envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_).RequestMatch.RequestTransformation.LogRequestResponseInfo = &wrapperspb.BoolValue{Value: true}
				output, err := p.(transformationPlugin).ConvertTransformation(
					ctx,
					&transformation.Transformations{},
					inputTransformationStages,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal(expectedOutput))
			})

			It("can override settings-object-level setting with transformation-stages level", func() {
				// override plugin created in BeforeEach
				p = NewPlugin()
				// initialize with settings-object-level setting enabled
				p.Init(plugins.InitParams{
					Ctx: ctx,
					Settings: &v1.Settings{
						Gloo: &v1.GlooOptions{
							RemoveUnusedFilters:            &wrapperspb.BoolValue{Value: false},
							TransformationEscapeCharacters: &wrapperspb.BoolValue{Value: true},
						},
					},
				})

				inputTransformationStages.LogRequestResponseInfo = &wrapperspb.BoolValue{Value: false}

				output, err := p.(transformationPlugin).ConvertTransformation(
					ctx,
					&transformation.Transformations{},
					inputTransformationStages,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal(expectedOutput))
			})
		})

		Context("EscapeCharacters", func() {
			var (
				inputTransformationStages *transformation.TransformationStages
				expectedOutput            *envoytransformation.RouteTransformations
				inputTransform            *transformation.Transformation
				outputTransform           *envoytransformation.Transformation
				True                      = &wrapperspb.BoolValue{Value: true}
				False                     = &wrapperspb.BoolValue{Value: false}
			)

			type transformationPlugin interface {
				plugins.Plugin
				ConvertTransformation(
					ctx context.Context,
					t *transformation.Transformations,
					stagedTransformations *transformation.TransformationStages,
				) (*envoytransformation.RouteTransformations, error)
			}

			BeforeEach(func() {
				inputTransform = &transformation.Transformation{
					TransformationType: &transformation.Transformation_TransformationTemplate{
						TransformationTemplate: &transformation.TransformationTemplate{},
					},
				}
				outputTransform = &envoytransformation.Transformation{
					TransformationType: &envoytransformation.Transformation_TransformationTemplate{
						TransformationTemplate: &envoytransformation.TransformationTemplate{},
					},
				}
				inputTransformationStages = &transformation.TransformationStages{
					Regular: &transformation.RequestResponseTransformations{
						RequestTransforms: []*transformation.RequestMatch{{
							RequestTransformation: inputTransform,
						}},
					},
				}

				expectedOutput = &envoytransformation.RouteTransformations{
					Transformations: []*envoytransformation.RouteTransformations_RouteTransformation{{
						Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
							RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
								RequestTransformation: outputTransform,
							},
						},
					}},
				}
			})

			It("can set escape_characters on transformation level", func() {
				inputTransform.GetTransformationTemplate().EscapeCharacters = True
				outputTransform.GetTransformationTemplate().EscapeCharacters = true

				output, err := p.(transformationPlugin).ConvertTransformation(
					ctx,
					&transformation.Transformations{},
					inputTransformationStages,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal(expectedOutput))
			})

			It("sets escape_characters to false if transformation-level setting is false", func() {
				inputTransform.GetTransformationTemplate().EscapeCharacters = False
				outputTransform.GetTransformationTemplate().EscapeCharacters = false

				output, err := p.(transformationPlugin).ConvertTransformation(
					ctx,
					&transformation.Transformations{},
					inputTransformationStages,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal(expectedOutput))
			})

			It("does not set escape_characters if transformation-level setting is nil", func() {
				inputTransform.GetTransformationTemplate().EscapeCharacters = False

				output, err := p.(transformationPlugin).ConvertTransformation(
					ctx,
					&transformation.Transformations{},
					inputTransformationStages,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal(expectedOutput))
			})

			It("can override transformation-stages level escape_characters with transformation level", func() {
				inputTransformationStages.Regular.RequestTransforms[0].RequestTransformation.GetTransformationTemplate().EscapeCharacters = False
				inputTransformationStages.EscapeCharacters = True
				expectedOutput.GetTransformations()[0].GetMatch().(*envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_).RequestMatch.GetRequestTransformation().GetTransformationTemplate().EscapeCharacters = false

				output, err := p.(transformationPlugin).ConvertTransformation(
					ctx,
					&transformation.Transformations{},
					inputTransformationStages,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal(expectedOutput))
			})

			It("can enable settings-object-level setting", func() {
				// initialize with settings-object-level setting enabled
				p.Init(plugins.InitParams{
					Ctx: ctx,
					Settings: &v1.Settings{
						Gloo: &v1.GlooOptions{
							RemoveUnusedFilters:            False,
							TransformationEscapeCharacters: True,
						},
					},
				})

				inputTransformationStages.Regular.RequestTransforms[0].RequestTransformation.GetTransformationTemplate().EscapeCharacters = nil
				inputTransformationStages.EscapeCharacters = nil
				expectedOutput.GetTransformations()[0].GetMatch().(*envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_).RequestMatch.GetRequestTransformation().GetTransformationTemplate().EscapeCharacters = true
				output, err := p.(transformationPlugin).ConvertTransformation(
					ctx,
					&transformation.Transformations{},
					inputTransformationStages,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal(expectedOutput))
			})

			FIt("can override settings-object-level setting with transformation-stages level", func() {
				// initialize with settings-object-level setting enabled
				p.Init(plugins.InitParams{
					Ctx: ctx,
					Settings: &v1.Settings{
						Gloo: &v1.GlooOptions{
							RemoveUnusedFilters:            False,
							TransformationEscapeCharacters: False,
						},
					},
				})

				inputTransformationStages.Regular.RequestTransforms[0].RequestTransformation.GetTransformationTemplate().EscapeCharacters = True
				inputTransformationStages.EscapeCharacters = nil
				expectedOutput.GetTransformations()[0].GetMatch().(*envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_).RequestMatch.GetRequestTransformation().GetTransformationTemplate().EscapeCharacters = true
				output, err := p.(transformationPlugin).ConvertTransformation(
					ctx,
					&transformation.Transformations{},
					inputTransformationStages,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal(expectedOutput))
			})

		})

	})

	Context("deprecated transformations", func() {
		var (
			inputTransform *transformation.Transformations
		)
		BeforeEach(func() {
			p = NewPlugin()
			p.Init(plugins.InitParams{Ctx: ctx, Settings: &v1.Settings{Gloo: &v1.GlooOptions{RemoveUnusedFilters: &wrapperspb.BoolValue{Value: false}}}})

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
			configStruct, err := utils.MessageToAny(outputTransform)
			Expect(err).NotTo(HaveOccurred())

			expected = configStruct
		})

		It("sets transformation config for weighted destinations", func() {
			out := &envoy_config_route_v3.WeightedCluster_ClusterWeight{}
			err := p.(plugins.WeightedDestinationPlugin).ProcessWeightedDestination(plugins.RouteParams{
				VirtualHostParams: plugins.VirtualHostParams{
					Params: plugins.Params{
						Ctx: ctx,
					},
				},
			}, &v1.WeightedDestination{
				Options: &v1.WeightedDestinationOptions{
					Transformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("repeatedly sets transformation config for virtual hosts", func() {
			out := &envoy_config_route_v3.VirtualHost{}
			err := p.(plugins.VirtualHostPlugin).ProcessVirtualHost(plugins.VirtualHostParams{
				Params: plugins.Params{
					Ctx: ctx,
				},
			}, &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					Transformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))

			out2 := &envoy_config_route_v3.VirtualHost{}
			err2 := p.(plugins.VirtualHostPlugin).ProcessVirtualHost(plugins.VirtualHostParams{
				Params: plugins.Params{
					Ctx: ctx,
				},
			}, &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					Transformations: inputTransform,
				},
			}, out2)
			Expect(err2).NotTo(HaveOccurred())
			Expect(out2.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("sets transformation config for routes", func() {
			out := &envoy_config_route_v3.Route{}
			err := p.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{
				VirtualHostParams: plugins.VirtualHostParams{
					Params: plugins.Params{
						Ctx: ctx,
					},
				},
			}, &v1.Route{
				Options: &v1.RouteOptions{
					Transformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("sets only one filter when no early filters exist", func() {
			filters, err := p.(plugins.HttpFilterPlugin).HttpFilters(plugins.Params{Ctx: ctx}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(filters)).To(Equal(1))
			value := filters[0].HttpFilter.GetTypedConfig().GetValue()
			Expect(value).To(BeEmpty())
		})
	})

	Context("staged transformations", func() {
		var (
			inputTransform         *transformation.TransformationStages
			earlyStageFilterConfig *any.Any
		)
		BeforeEach(func() {
			p = NewPlugin()
			p.Init(plugins.InitParams{Ctx: ctx, Settings: &v1.Settings{Gloo: &v1.GlooOptions{RemoveUnusedFilters: &wrapperspb.BoolValue{Value: false}}}})

			var err error
			earlyStageFilterConfig, err = utils.MessageToAny(&envoytransformation.FilterTransformations{
				Stage: EarlyStageNumber,
			})
			Expect(err).NotTo(HaveOccurred())
			earlyRequestTransformationTemplateIn := &transformation.TransformationTemplate{
				AdvancedTemplates: true,
				BodyTransformation: &transformation.TransformationTemplate_Body{
					Body: &transformation.InjaTemplate{Text: "1"},
				},
			}
			earlyRequestTransformationTemplate := &envoytransformation.TransformationTemplate{
				AdvancedTemplates: true,
				BodyTransformation: &envoytransformation.TransformationTemplate_Body{
					Body: &envoytransformation.InjaTemplate{Text: "1"},
				},
			}
			// construct transformation with all the options, to make sure translation is correct
			earlyRequestTransform := &transformation.Transformation{
				TransformationType: &transformation.Transformation_TransformationTemplate{
					TransformationTemplate: earlyRequestTransformationTemplateIn,
				},
			}
			envoyEarlyRequestTransform := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_TransformationTemplate{
					TransformationTemplate: earlyRequestTransformationTemplate,
				},
			}
			earlyResponseTransformationTemplateIn := &transformation.TransformationTemplate{
				AdvancedTemplates: true,
				BodyTransformation: &transformation.TransformationTemplate_Body{
					Body: &transformation.InjaTemplate{Text: "2"},
				},
			}
			earlyResponseTransformationTemplate := &envoytransformation.TransformationTemplate{
				AdvancedTemplates: true,
				BodyTransformation: &envoytransformation.TransformationTemplate_Body{
					Body: &envoytransformation.InjaTemplate{Text: "2"},
				},
			}
			earlyResponseTransform := &transformation.Transformation{
				TransformationType: &transformation.Transformation_TransformationTemplate{
					TransformationTemplate: earlyResponseTransformationTemplateIn,
				},
			}
			envoyEarlyResponseTransform := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_TransformationTemplate{
					TransformationTemplate: earlyResponseTransformationTemplate,
				},
			}
			requestTransformationIn := &transformation.TransformationTemplate{
				AdvancedTemplates: true,
				BodyTransformation: &transformation.TransformationTemplate_Body{
					Body: &transformation.InjaTemplate{Text: "11"},
				},
			}
			requestTransformation := &envoytransformation.TransformationTemplate{
				AdvancedTemplates: true,
				BodyTransformation: &envoytransformation.TransformationTemplate_Body{
					Body: &envoytransformation.InjaTemplate{Text: "11"},
				},
			}
			requestTransform := &transformation.Transformation{
				TransformationType: &transformation.Transformation_TransformationTemplate{
					TransformationTemplate: requestTransformationIn,
				},
			}
			envoyRequestTransform := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_TransformationTemplate{
					TransformationTemplate: requestTransformation,
				},
			}
			responseTransformationIn := &transformation.TransformationTemplate{
				AdvancedTemplates: true,
				BodyTransformation: &transformation.TransformationTemplate_Body{
					Body: &transformation.InjaTemplate{Text: "12"},
				},
			}
			responseTransformation := &envoytransformation.TransformationTemplate{
				AdvancedTemplates: true,
				BodyTransformation: &envoytransformation.TransformationTemplate_Body{
					Body: &envoytransformation.InjaTemplate{Text: "12"},
				},
			}
			responseTransform := &transformation.Transformation{
				TransformationType: &transformation.Transformation_TransformationTemplate{
					TransformationTemplate: responseTransformationIn,
				},
			}
			envoyResponseTransform := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_TransformationTemplate{
					TransformationTemplate: responseTransformation,
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
								ResponseTransformation: envoyEarlyResponseTransform,
							},
						},
					},
					{
						Stage: EarlyStageNumber,
						Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
							RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
								Match:                  &v3.RouteMatch{PathSpecifier: &v3.RouteMatch_Prefix{Prefix: "/foo"}},
								ClearRouteCache:        true,
								RequestTransformation:  envoyEarlyRequestTransform,
								ResponseTransformation: envoyEarlyResponseTransform,
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
								ResponseTransformation: envoyEarlyResponseTransform,
							},
						},
					},
					{
						Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
							RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
								Match:                  &v3.RouteMatch{PathSpecifier: &v3.RouteMatch_Prefix{Prefix: "/foo2"}},
								ClearRouteCache:        true,
								RequestTransformation:  envoyRequestTransform,
								ResponseTransformation: envoyResponseTransform,
							},
						},
					},
				},
			}
			configStruct, err := utils.MessageToAny(outputTransform)
			Expect(err).NotTo(HaveOccurred())

			expected = configStruct
		})
		It("sets transformation config for vhosts", func() {
			out := &envoy_config_route_v3.VirtualHost{}
			err := p.(plugins.VirtualHostPlugin).ProcessVirtualHost(plugins.VirtualHostParams{
				Params: plugins.Params{
					Ctx: ctx,
				},
			}, &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					StagedTransformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("sets transformation config for routes", func() {
			out := &envoy_config_route_v3.Route{}
			err := p.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{
				VirtualHostParams: plugins.VirtualHostParams{
					Params: plugins.Params{
						Ctx: ctx,
					},
				},
			}, &v1.Route{
				Options: &v1.RouteOptions{
					StagedTransformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("sets transformation config for weighted dest", func() {
			out := &envoy_config_route_v3.WeightedCluster_ClusterWeight{}
			err := p.(plugins.WeightedDestinationPlugin).ProcessWeightedDestination(plugins.RouteParams{
				VirtualHostParams: plugins.VirtualHostParams{
					Params: plugins.Params{
						Ctx: ctx,
					},
				},
			}, &v1.WeightedDestination{
				Options: &v1.WeightedDestinationOptions{
					StagedTransformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("should add both filter to the chain when early transformations exist", func() {
			out := &envoy_config_route_v3.Route{}
			err := p.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: plugins.VirtualHostParams{
				Params: plugins.Params{
					Ctx: ctx,
				},
			}}, &v1.Route{
				Options: &v1.RouteOptions{
					StagedTransformations: inputTransform,
				},
			}, out)
			filters, err := p.(plugins.HttpFilterPlugin).HttpFilters(plugins.Params{Ctx: ctx}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(filters)).To(Equal(2))
			value := filters[0].HttpFilter.GetTypedConfig()
			Expect(value).To(Equal(earlyStageFilterConfig))
			// second filter should have no stage, and thus empty config
			value = filters[1].HttpFilter.GetTypedConfig()
			Expect(value.GetValue()).To(BeEmpty())
		})
	})

})
