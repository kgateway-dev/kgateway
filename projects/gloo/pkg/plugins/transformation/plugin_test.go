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
							RemoveUnusedFilters:                  &wrapperspb.BoolValue{Value: false},
							LogTransformationRequestResponseInfo: &wrapperspb.BoolValue{Value: true},
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
							RemoveUnusedFilters:                  &wrapperspb.BoolValue{Value: false},
							LogTransformationRequestResponseInfo: &wrapperspb.BoolValue{Value: true},
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

			It("can override settings-object-level setting with transformation-stages level", func() {
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

		Context("Extractors", func() {
			// Helper functions to create input and output transformations
			createInputExtraction := func(regex string, mode transformation.Extraction_Mode, subgroup uint32, replacementText *wrapperspb.StringValue) *transformation.Extraction {
				return &transformation.Extraction{
					Regex:           regex,
					Mode:            mode,
					Subgroup:        subgroup,
					ReplacementText: replacementText,
					Source:          &transformation.Extraction_Header{Header: "foo"},
				}
			}

			createOutputExtraction := func(regex string, mode envoytransformation.Extraction_Mode, subgroup uint32, replacementText *wrapperspb.StringValue) *envoytransformation.Extraction {
				return &envoytransformation.Extraction{
					Regex:           regex,
					Mode:            mode,
					Subgroup:        subgroup,
					ReplacementText: replacementText,
					Source:          &envoytransformation.Extraction_Header{Header: "foo"},
				}
			}

			// TODO: we can probably remove some layers of indirection + get rid of some of these helpers
			// intermediary function to create a transformation with a single extraction
			createInputTransformation := func(extraction *transformation.Extraction) *transformation.Transformation {
				return &transformation.Transformation{
					TransformationType: &transformation.Transformation_TransformationTemplate{
						TransformationTemplate: &transformation.TransformationTemplate{
							Extractors: map[string]*transformation.Extraction{"foo": extraction},
						},
					},
				}
			}

			// intermediary function to create a transformation with a single extraction
			createOutputTransformation := func(extraction *envoytransformation.Extraction) *envoytransformation.Transformation {
				return &envoytransformation.Transformation{
					TransformationType: &envoytransformation.Transformation_TransformationTemplate{
						TransformationTemplate: &envoytransformation.TransformationTemplate{
							Extractors: map[string]*envoytransformation.Extraction{"foo": extraction},
						},
					},
				}
			}

			// the output of this function can be compared directly with the output of the plugin
			createOutputRouteTransformations := func(transformation *envoytransformation.Transformation) *envoytransformation.RouteTransformations {
				return &envoytransformation.RouteTransformations{
					Transformations: []*envoytransformation.RouteTransformations_RouteTransformation{{
						Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
							RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
								RequestTransformation: transformation,
							},
						},
					}},
				}
			}

			// helper that creates a RouteTransformations with a single extraction, using the intermediary functions above
			createOutputRouteTransformationsFromExtraction := func(extraction *envoytransformation.Extraction) *envoytransformation.RouteTransformations {
				return createOutputRouteTransformations(createOutputTransformation(extraction))
			}

			// we this custom comparison because generated protos can't be compared directly
			validateExtractionMatch := func(expected, actual *envoytransformation.RouteTransformations) {
				getTransformation := func(rt *envoytransformation.RouteTransformations) *envoytransformation.Transformation {
					return rt.GetTransformations()[0].GetMatch().(*envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_).RequestMatch.GetRequestTransformation()
				}

				getExtractor := func(t *envoytransformation.Transformation, key string) *envoytransformation.Extraction {
					return t.GetTransformationTemplate().GetExtractors()[key]
				}

				expectedTransformation := getTransformation(expected)
				actualTransformation := getTransformation(actual)
				expectedExtraction := getExtractor(expectedTransformation, "foo")
				actualExtraction := getExtractor(actualTransformation, "foo")

				// Validate each field with proper nil checks
				Expect(actualExtraction.GetSource()).To(Equal(expectedExtraction.GetSource()), "Source mismatch")
				Expect(actualExtraction.GetRegex()).To(Equal(expectedExtraction.GetRegex()), "Regex mismatch")
				Expect(actualExtraction.GetSubgroup()).To(Equal(expectedExtraction.GetSubgroup()), "Subgroup mismatch")
				Expect(actualExtraction.GetMode()).To(Equal(expectedExtraction.GetMode()), "Mode mismatch")

				// Handle nil replacement text gracefully
				if expectedExtraction.GetReplacementText() != nil {
					Expect(actualExtraction.GetReplacementText()).NotTo(BeNil(), "Expected replacement text not to be nil")
					Expect(actualExtraction.GetReplacementText().GetValue()).To(Equal(expectedExtraction.GetReplacementText().GetValue()), "Replacement text value mismatch")
				} else {
					Expect(actualExtraction.GetReplacementText()).To(BeNil(), "Expected replacement text to be nil")
				}
			}

			type transformationPlugin interface {
				plugins.Plugin
				ConvertTransformation(
					ctx context.Context,
					t *transformation.Transformations,
					stagedTransformations *transformation.TransformationStages,
				) (*envoytransformation.RouteTransformations, error)
			}

			type extractorTestCase struct {
				Regex           string
				Mode            transformation.Extraction_Mode
				Subgroup        uint32
				ReplacementText string // Use an empty string to represent nil
				ExpectError     bool
			}

			DescribeTable("Extractor transformations",
				func(tc extractorTestCase) {
					var replacementText *wrapperspb.StringValue
					if tc.ReplacementText != "" {
						replacementText = &wrapperspb.StringValue{Value: tc.ReplacementText}
					}

					inputExtraction := createInputExtraction(tc.Regex, tc.Mode, tc.Subgroup, replacementText)
					inputTransform := createInputTransformation(inputExtraction)
					inputTransformationStages := &transformation.TransformationStages{
						Regular: &transformation.RequestResponseTransformations{
							RequestTransforms: []*transformation.RequestMatch{{
								RequestTransformation: inputTransform,
							}},
						},
					}

					output, err := p.(transformationPlugin).ConvertTransformation(ctx, &transformation.Transformations{}, inputTransformationStages)

					if tc.ExpectError {
						Expect(err).To(HaveOccurred())
					} else {
						Expect(err).NotTo(HaveOccurred())
						expectedOutputExtraction := createOutputExtraction(tc.Regex, envoytransformation.Extraction_Mode(tc.Mode), tc.Subgroup, replacementText)
						expectedOutput := createOutputRouteTransformationsFromExtraction(expectedOutputExtraction)
						validateExtractionMatch(expectedOutput, output)
					}
				},

				// Extract Mode Test Cases
				// TODO: this one doesn't quite work in the table setup
				Entry("Defaults to Extract mode",
					extractorTestCase{
						Regex:           "abc",
						Mode:            transformation.Extraction_EXTRACT,
						Subgroup:        0,
						ReplacementText: "",
						ExpectError:     false,
					},
				),
				Entry("Errors if subgroup is larger than number of capture groups in regex - Extract mode",
					extractorTestCase{
						Regex:           "(abc)",
						Mode:            transformation.Extraction_EXTRACT,
						Subgroup:        2,
						ReplacementText: "",
						ExpectError:     true,
					},
				),
				Entry("Errors if replacement_text is set - Extract mode",
					extractorTestCase{
						Regex:           "abc",
						Mode:            transformation.Extraction_EXTRACT,
						Subgroup:        0,
						ReplacementText: "replacement",
						ExpectError:     true,
					},
				),

				// Single Replace Mode Test Cases
				Entry("Can set mode to Single Replace with valid replacement text",
					extractorTestCase{
						Regex:           "abc",
						Mode:            transformation.Extraction_SINGLE_REPLACE,
						Subgroup:        0,
						ReplacementText: "foo",
						ExpectError:     false,
					},
				),
				Entry("Errors if replacement_text is not set in Single Replace mode",
					extractorTestCase{
						Regex:           "abc",
						Mode:            transformation.Extraction_SINGLE_REPLACE,
						Subgroup:        0,
						ReplacementText: "",
						ExpectError:     true,
					},
				),
				Entry("Errors if subgroup is larger than number of capture groups in regex - Single Replace mode",
					extractorTestCase{
						Regex:           "(abc)",
						Mode:            transformation.Extraction_SINGLE_REPLACE,
						Subgroup:        2,
						ReplacementText: "foo",
						ExpectError:     true,
					},
				),

				// Replace All Mode Test Cases
				Entry("Can set mode to ReplaceAll with valid replacement text",
					extractorTestCase{
						Regex:           "abc",
						Mode:            transformation.Extraction_REPLACE_ALL,
						Subgroup:        0,
						ReplacementText: "foo",
						ExpectError:     false,
					},
				),
				Entry("Errors if subgroup is set - Replace All mode",
					extractorTestCase{
						Regex:           "abc",
						Mode:            transformation.Extraction_REPLACE_ALL,
						Subgroup:        1,
						ReplacementText: "foo",
						ExpectError:     true,
					},
				),
				Entry("Errors if replacement_text is not set - Replace All mode",
					extractorTestCase{
						Regex:           "abc",
						Mode:            transformation.Extraction_REPLACE_ALL,
						Subgroup:        0,
						ReplacementText: "",
						ExpectError:     true,
					},
				),
			)
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
			Expect(filters).To(HaveLen(1))
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
			Expect(err).NotTo(HaveOccurred())
			filters, err := p.(plugins.HttpFilterPlugin).HttpFilters(plugins.Params{Ctx: ctx}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(filters).To(HaveLen(2))
			value := filters[0].HttpFilter.GetTypedConfig()
			Expect(value).To(Equal(earlyStageFilterConfig))
			// second filter should have no stage, and thus empty config
			value = filters[1].HttpFilter.GetTypedConfig()
			Expect(value.GetValue()).To(BeEmpty())
		})
	})

})
