package transformation

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/solo-io/gloo/pkg/utils/statsutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/validator"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/solo-io/gloo/pkg/utils/regexutils"
	envoyroutev3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/route/v3"
	envoytransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	upstream_wait "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/upstream_wait"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	proto_utils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var (
	_ plugins.Plugin                    = new(Plugin)
	_ plugins.VirtualHostPlugin         = new(Plugin)
	_ plugins.WeightedDestinationPlugin = new(Plugin)
	_ plugins.RoutePlugin               = new(Plugin)
	_ plugins.HttpFilterPlugin          = new(Plugin)
	_ plugins.UpstreamHttpFilterPlugin  = new(Plugin)
)

const (
	ExtensionName      = "transformation"
	FilterName         = "io.solo.transformation"
	WaitFilterName     = "io.solo.wait"
	RegularStageNumber = 0
	EarlyStageNumber   = 1
	AwsStageNumber     = 2
	PostRoutingNumber  = 3
)

var (
	earlyPluginStage = plugins.AfterStage(plugins.FaultStage)
	pluginStage      = plugins.AfterStage(plugins.AuthZStage)

	UnknownTransformationType = func(transformation interface{}) error {
		return fmt.Errorf("unknown transformation type %T", transformation)
	}
)

type TranslateTransformationFn func(*transformation.Transformation, *wrapperspb.BoolValue, *wrapperspb.BoolValue) (*envoytransformation.Transformation, error)

// This Plugin is exported only because it is utilized by the enterprise implementation
// We would prefer if the plugin were not exported and instead the required translation
// methods were exported.
type Plugin struct {
	removeUnused                      bool
	filterRequiredForListener         map[*v1.HttpListener]struct{}
	upstreamFilterRequiredForListener map[*v1.HttpListener]struct{}

	RequireEarlyTransformation bool
	TranslateTransformation    TranslateTransformationFn
	settings                   *v1.Settings
	logRequestResponseInfo     bool
	escapeCharacters           *wrapperspb.BoolValue
	validator                  validator.Validator
}

func NewPlugin() *Plugin {
	mCacheHits := statsutils.MakeSumCounter("gloo.solo.io/transformation_validation_cache_hits", "The number of cache hits while validating transformation config")
	mCacheMisses := statsutils.MakeSumCounter("gloo.solo.io/transformation_validation_cache_misses", "The number of cache misses while validating transformation config")

	return &Plugin{
		validator: validator.New(ExtensionName, FilterName,
			validator.WithCounters(mCacheHits, mCacheMisses)),
	}
}

func (p *Plugin) Name() string {
	return ExtensionName
}

// Init attempts to set the plugin back to a clean slate state.
func (p *Plugin) Init(params plugins.InitParams) {
	p.RequireEarlyTransformation = false
	p.removeUnused = params.Settings.GetGloo().GetRemoveUnusedFilters().GetValue()
	p.filterRequiredForListener = make(map[*v1.HttpListener]struct{})
	p.upstreamFilterRequiredForListener = make(map[*v1.HttpListener]struct{})
	p.settings = params.Settings
	p.TranslateTransformation = TranslateTransformation
	p.escapeCharacters = params.Settings.GetGloo().GetTransformationEscapeCharacters()
	p.logRequestResponseInfo = params.Settings.GetGloo().GetLogTransformationRequestResponseInfo().GetValue()
}

func mergeFunc(tx *envoytransformation.RouteTransformations) pluginutils.ModifyFunc {
	return func(existing *any.Any) (proto.Message, error) {
		if existing == nil {
			return tx, nil
		}
		var transforms envoytransformation.RouteTransformations
		err := existing.UnmarshalTo(&transforms)
		if err != nil {
			// this should never happen
			return nil, err
		}
		transforms.Transformations = append(transforms.GetTransformations(), tx.GetTransformations()...)
		return &transforms, nil
	}
}

func (p *Plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {
	envoyTransformation, err := p.ConvertTransformation(
		params.Ctx,
		in.GetOptions().GetTransformations(),
		in.GetOptions().GetStagedTransformations(),
	)
	if err != nil {
		return err
	}
	if envoyTransformation == nil {
		return nil
	}
	err = p.validateTransformation(params.Ctx, envoyTransformation)
	if err != nil {
		return err
	}

	p.filterRequiredForListener[params.HttpListener] = struct{}{}
	p.requiresUpstreamFilter(params.HttpListener, in.GetOptions().GetStagedTransformations())

	return pluginutils.ModifyVhostPerFilterConfig(out, FilterName, mergeFunc(envoyTransformation))
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	envoyTransformation, err := p.ConvertTransformation(
		params.Ctx,
		in.GetOptions().GetTransformations(),
		in.GetOptions().GetStagedTransformations(),
	)
	if err != nil {
		return err
	}
	if envoyTransformation == nil {
		return nil
	}
	err = p.validateTransformation(params.Ctx, envoyTransformation)
	if err != nil {
		return err
	}

	p.filterRequiredForListener[params.HttpListener] = struct{}{}
	p.requiresUpstreamFilter(params.HttpListener, in.GetOptions().GetStagedTransformations())

	return pluginutils.ModifyRoutePerFilterConfig(out, FilterName, mergeFunc(envoyTransformation))
}

func (p *Plugin) ProcessWeightedDestination(
	params plugins.RouteActionParams,
	in *v1.WeightedDestination,
	out *envoy_config_route_v3.WeightedCluster_ClusterWeight,
) error {
	envoyTransformation, err := p.ConvertTransformation(
		params.Ctx,
		in.GetOptions().GetTransformations(),
		in.GetOptions().GetStagedTransformations(),
	)
	if err != nil {
		return err
	}
	if envoyTransformation == nil {
		return nil
	}

	err = p.validateTransformation(params.Ctx, envoyTransformation)
	if err != nil {
		return err
	}

	p.filterRequiredForListener[params.HttpListener] = struct{}{}
	p.requiresUpstreamFilter(params.HttpListener, in.GetOptions().GetStagedTransformations())

	return pluginutils.ModifyWeightedClusterPerFilterConfig(out, FilterName, mergeFunc(envoyTransformation))
}

// HttpFilters emits the desired set of filters. Either 0, 1 or
// if earlytransformation is needed then 2 staged filters
func (p *Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	var filters []plugins.StagedHttpFilter

	_, ok := p.filterRequiredForListener[listener]
	if !ok && p.removeUnused {
		return filters, nil
	}

	if p.RequireEarlyTransformation {
		// only add early transformations if we have to, to allow rolling gloo updates;
		// i.e. an older envoy without stages connects to gloo, it shouldn't have 2 filters.
		earlyStageConfig := &envoytransformation.FilterTransformations{
			Stage:                  EarlyStageNumber,
			LogRequestResponseInfo: p.logRequestResponseInfo,
		}
		earlyFilter, err := plugins.NewStagedFilter(FilterName, earlyStageConfig, earlyPluginStage)
		if err != nil {
			return nil, err
		}
		filters = append(filters, earlyFilter)
	}

	filters = append(filters,
		plugins.MustNewStagedFilter(FilterName,
			&envoytransformation.FilterTransformations{
				LogRequestResponseInfo: p.logRequestResponseInfo,
			},
			pluginStage),
	)

	return filters, nil
}

func (p *Plugin) requiresUpstreamFilter(listener *v1.HttpListener, stagedTransformations *transformation.TransformationStages) {
	if stagedTransformations.GetPostRouting() != nil {
		p.upstreamFilterRequiredForListener[listener] = struct{}{}
	}
}

func (p *Plugin) UpstreamHttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedUpstreamHttpFilter, error) {
	if _, ok := p.upstreamFilterRequiredForListener[listener]; !ok {
		return nil, nil
	}

	transformationMsg, err := proto_utils.MessageToAny(&envoytransformation.FilterTransformations{
		LogRequestResponseInfo: p.logRequestResponseInfo,
		Stage:                  PostRoutingNumber,
	})
	if err != nil {
		return nil, err
	}

	upstreamWaitMsg, err := proto_utils.MessageToAny(&upstream_wait.UpstreamWaitFilterConfig{})
	if err != nil {
		return nil, err
	}

	filters := []plugins.StagedUpstreamHttpFilter{
		// The wait filter essentially blocks filter iteration until a host has been selected.
		// This is important because running as an upstream filter allows access to host
		// metadata iff the host has already been selected, and that's a
		// major benefit of running the filter at this stage.
		{
			Filter: &envoyhttp.HttpFilter{
				Name: WaitFilterName,
				ConfigType: &envoyhttp.HttpFilter_TypedConfig{
					TypedConfig: upstreamWaitMsg,
				},
			},
			Stage: plugins.UpstreamHTTPFilterStage{
				RelativeTo: plugins.TransformationStage,
				Weight:     -1,
			},
		},
		{
			Filter: &envoyhttp.HttpFilter{
				Name: FilterName,
				ConfigType: &envoyhttp.HttpFilter_TypedConfig{
					TypedConfig: transformationMsg,
				},
			},
			Stage: plugins.UpstreamHTTPFilterStage{
				RelativeTo: plugins.TransformationStage,
				Weight:     0,
			},
		},
	}

	return filters, nil

}

func (p *Plugin) ConvertTransformation(
	ctx context.Context,
	t *transformation.Transformations,
	stagedTransformations *transformation.TransformationStages,
) (*envoytransformation.RouteTransformations, error) {
	if t == nil && stagedTransformations == nil {
		return nil, nil
	}
	ret := &envoytransformation.RouteTransformations{}
	if t != nil && stagedTransformations.GetRegular() == nil {
		// keep deprecated config until we are sure we don't need it.
		// on newer envoys it will be ignored.
		requestTransform, err := p.TranslateTransformation(t.GetRequestTransformation(), p.escapeCharacters, nil)
		if err != nil {
			return nil, err
		}
		responseTransform, err := p.TranslateTransformation(t.GetResponseTransformation(), p.escapeCharacters, nil)
		if err != nil {
			return nil, err
		}

		ret.RequestTransformation = requestTransform
		ret.ClearRouteCache = t.GetClearRouteCache()
		ret.ResponseTransformation = responseTransform
		// new config:
		// we have to have it too, as if any new config is defined the deprecated config is ignored.
		ret.Transformations = append(ret.GetTransformations(),
			&envoytransformation.RouteTransformations_RouteTransformation{
				Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
					RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
						Match:                  nil,
						RequestTransformation:  requestTransform,
						ClearRouteCache:        t.GetClearRouteCache(),
						ResponseTransformation: responseTransform,
					},
				},
			})
	}

	stagedEscapeCharacters := stagedTransformations.GetEscapeCharacters()
	if early := stagedTransformations.GetEarly(); early != nil {
		p.RequireEarlyTransformation = true
		transformations, err := p.getTransformations(ctx, EarlyStageNumber, early, stagedEscapeCharacters)
		if err != nil {
			return nil, err
		}
		ret.Transformations = append(ret.GetTransformations(), transformations...)
	}
	if regular := stagedTransformations.GetRegular(); regular != nil {
		transformations, err := p.getTransformations(ctx, RegularStageNumber, regular, stagedEscapeCharacters)
		if err != nil {
			return nil, err
		}
		ret.Transformations = append(ret.GetTransformations(), transformations...)
	}
	if postRouting := stagedTransformations.GetPostRouting(); postRouting != nil {
		transformations, err := p.getTransformations(ctx, PostRoutingNumber, postRouting, stagedEscapeCharacters)
		if err != nil {
			return nil, err
		}
		ret.Transformations = append(ret.GetTransformations(), transformations...)
	}

	// if the global settings or the route/vhost settings are set to true, set all transformation-level settings to true
	// note that the route/vhost settings take precedence over the global settings
	if stagedTransformations.GetLogRequestResponseInfo().GetValue() ||
		(stagedTransformations.GetLogRequestResponseInfo() == nil && p.logRequestResponseInfo) {
		for _, t := range ret.GetTransformations() {
			if requestMatch := t.GetRequestMatch(); requestMatch != nil {
				if requestTransformation := requestMatch.GetRequestTransformation(); requestTransformation != nil {
					requestTransformation.LogRequestResponseInfo = &wrapperspb.BoolValue{Value: true}
				}
				if responseTransformation := requestMatch.GetResponseTransformation(); responseTransformation != nil {
					responseTransformation.LogRequestResponseInfo = &wrapperspb.BoolValue{Value: true}
				}
			}
			if responseMatch := t.GetResponseMatch(); responseMatch != nil {
				if responseTransformation := responseMatch.GetResponseTransformation(); responseTransformation != nil {
					responseTransformation.LogRequestResponseInfo = &wrapperspb.BoolValue{Value: true}
				}
			}
		}
	}

	return ret, nil
}

func TranslateTransformation(glooTransform *transformation.Transformation,
	settingsEscapeCharacters *wrapperspb.BoolValue,
	stagedEscapeCharacters *wrapperspb.BoolValue) (*envoytransformation.Transformation, error) {

	if glooTransform == nil {
		return nil, nil
	}
	out := &envoytransformation.Transformation{}

	switch typedTransformation := glooTransform.GetTransformationType().(type) {
	case *transformation.Transformation_HeaderBodyTransform:
		{
			out.TransformationType = translateHeaderBodyTransform(typedTransformation)
		}
	case *transformation.Transformation_TransformationTemplate:
		{
			escapeCharacters := typedTransformation.TransformationTemplate.GetEscapeCharacters()
			if escapeCharacters == nil {
				escapeCharacters = stagedEscapeCharacters
			}
			if escapeCharacters == nil {
				escapeCharacters = settingsEscapeCharacters
			}
			typedTransformation.TransformationTemplate.EscapeCharacters = escapeCharacters

			transformationType, err := translateTransformationTemplate(typedTransformation)
			if err != nil {
				return nil, err
			}
			out.TransformationType = transformationType
		}
	default:
		return nil, UnknownTransformationType(typedTransformation)
	}

	// this is the transformation-level logRequestResponseInfo setting
	if glooTransform.GetLogRequestResponseInfo() {
		out.LogRequestResponseInfo = &wrapperspb.BoolValue{Value: true}
	}

	return out, nil
}

func translateHeaderBodyTransform(in *transformation.Transformation_HeaderBodyTransform) *envoytransformation.Transformation_HeaderBodyTransform {
	out := &envoytransformation.Transformation_HeaderBodyTransform{}
	out.HeaderBodyTransform = &envoytransformation.HeaderBodyTransform{
		AddRequestMetadata: in.HeaderBodyTransform.GetAddRequestMetadata(),
	}
	return out
}

func translateTransformationTemplate(in *transformation.Transformation_TransformationTemplate) (*envoytransformation.Transformation_TransformationTemplate, error) {
	out := &envoytransformation.Transformation_TransformationTemplate{}
	inTemplate := in.TransformationTemplate
	outTemplate := &envoytransformation.TransformationTemplate{
		AdvancedTemplates:  inTemplate.GetAdvancedTemplates(),
		HeadersToRemove:    inTemplate.GetHeadersToRemove(),
		IgnoreErrorOnParse: inTemplate.GetIgnoreErrorOnParse(),
		ParseBodyBehavior:  envoytransformation.TransformationTemplate_RequestBodyParse(inTemplate.GetParseBodyBehavior()),
		EscapeCharacters:   inTemplate.GetEscapeCharacters().GetValue(), // the inheritance is handled in TranslateTransformation
	}

	if len(inTemplate.GetExtractors()) > 0 {
		outTemplate.Extractors = make(map[string]*envoytransformation.Extraction)
		for k, v := range inTemplate.GetExtractors() {
			extractor, err := translateExtractor(v, k)
			if err != nil {
				return nil, err
			}
			outTemplate.GetExtractors()[k] = extractor
		}
	}

	if len(inTemplate.GetHeaders()) > 0 {
		outTemplate.Headers = make(map[string]*envoytransformation.InjaTemplate)
		for k, v := range inTemplate.GetHeaders() {
			outTemplate.GetHeaders()[k] = &envoytransformation.InjaTemplate{Text: v.GetText()}
		}
	}

	if len(inTemplate.GetHeadersToAppend()) > 0 {
		outTemplate.HeadersToAppend = make(
			[]*envoytransformation.TransformationTemplate_HeaderToAppend,
			len(inTemplate.GetHeadersToAppend()))
		headers := inTemplate.GetHeadersToAppend()
		for i := range headers {
			outTemplate.GetHeadersToAppend()[i] = &envoytransformation.TransformationTemplate_HeaderToAppend{
				Key:   headers[i].GetKey(),
				Value: &envoytransformation.InjaTemplate{Text: headers[i].GetValue().GetText()},
			}

		}
	}

	switch bodyTransformation := inTemplate.GetBodyTransformation().(type) {
	case *transformation.TransformationTemplate_Body:
		outTemplate.BodyTransformation = &envoytransformation.TransformationTemplate_Body{
			Body: &envoytransformation.InjaTemplate{
				Text: bodyTransformation.Body.GetText(),
			},
		}
	case *transformation.TransformationTemplate_Passthrough:
		outTemplate.BodyTransformation = &envoytransformation.TransformationTemplate_Passthrough{
			Passthrough: &envoytransformation.Passthrough{},
		}
	case *transformation.TransformationTemplate_MergeExtractorsToBody:
		outTemplate.BodyTransformation = &envoytransformation.TransformationTemplate_MergeExtractorsToBody{
			MergeExtractorsToBody: &envoytransformation.MergeExtractorsToBody{},
		}
	case *transformation.TransformationTemplate_MergeJsonKeys:
		if inTemplate.GetAdvancedTemplates() {
			return nil, fmt.Errorf("merge_json_keys is not supported with advanced templates")
		}
		if inTemplate.GetParseBodyBehavior() != transformation.TransformationTemplate_ParseAsJson {
			return nil, fmt.Errorf("merge_json_keys is only supported with parse_body_behavior set to PARSE_AS_JSON")
		}
		jsonKeys := make(map[string]*envoytransformation.MergeJsonKeys_OverridableTemplate, len(bodyTransformation.MergeJsonKeys.GetJsonKeys()))
		for key, val := range bodyTransformation.MergeJsonKeys.GetJsonKeys() {
			if strings.Contains(key, ".") {
				return nil, fmt.Errorf("merge_json_keys key %s contains a period, which is not currently supported", key)
			}
			jsonKeys[key] = &envoytransformation.MergeJsonKeys_OverridableTemplate{
				Tmpl:          &envoytransformation.InjaTemplate{Text: val.GetTmpl().GetText()},
				OverrideEmpty: val.GetOverrideEmpty(),
			}
		}
		outTemplate.BodyTransformation = &envoytransformation.TransformationTemplate_MergeJsonKeys{
			MergeJsonKeys: &envoytransformation.MergeJsonKeys{
				JsonKeys: jsonKeys,
			},
		}
	}

	if len(inTemplate.GetDynamicMetadataValues()) > 0 {
		outTemplate.DynamicMetadataValues = make([]*envoytransformation.TransformationTemplate_DynamicMetadataValue, len(inTemplate.GetDynamicMetadataValues()))
		values := inTemplate.GetDynamicMetadataValues()
		for i := range values {
			outTemplate.GetDynamicMetadataValues()[i] = &envoytransformation.TransformationTemplate_DynamicMetadataValue{
				MetadataNamespace: values[i].GetMetadataNamespace(),
				Key:               values[i].GetKey(),
				JsonToProto:       values[i].GetJsonToProto(),
				Value: &envoytransformation.InjaTemplate{
					Text: values[i].GetValue().GetText(),
				},
			}
		}
	}

	if spanTransformer := inTemplate.GetSpanTransformer(); spanTransformer != nil {
		outTemplate.SpanTransformer = &envoytransformation.TransformationTemplate_SpanTransformer{
			Name: &envoytransformation.InjaTemplate{Text: spanTransformer.GetName().GetText()},
		}
	}

	out.TransformationTemplate = outTemplate
	return out, nil
}

// ExtractorError represents an error related to extractor configuration.
type ExtractorError struct {
	Message string // The error message
	Name    string // The name of the extractor causing the error
	Mode    string // (optional) The (stringified) mode of the extractor causing the error
}

// implements the error interface for ExtractorError.
func (e *ExtractorError) Error() string {
	if e.Mode == "" {
		return fmt.Sprintf("%s for extractor %s", e.Message, e.Name)
	} else {
		return fmt.Sprintf("%s for extractor %s in mode %s", e.Message, e.Name, e.Mode)
	}
}

// Helper functions to create specific ExtractorError instances.
func NewExtractorError(message, name string, mode transformation.Extraction_Mode) *ExtractorError {
	extractorError := &ExtractorError{
		Message: message,
		Name:    name,
	}

	// check if there's a readable mode name
	if modeName, ok := transformation.Extraction_Mode_name[int32(mode)]; ok {
		extractorError.Mode = modeName
	}
	return extractorError
}

func NewInvalidSpanTransformerError(message string) error {
	return fmt.Errorf("unable to create span transformer: %s", message)
}

const (
	ErrMsgReplacementTextSetWhenNotNeeded = "replacement text should not be set"
	ErrMsgReplacementTextNotSetWhenNeeded = "replacement text must be set"
	ErrMsgSubgroupSetWhenNotNeeded        = "subgroup should not be set"
)

func translateExtractor(extractor *transformation.Extraction, name string) (*envoytransformation.Extraction, error) {
	out := &envoytransformation.Extraction{
		Regex: extractor.GetRegex(),
	}

	switch src := extractor.GetSource().(type) {
	case *transformation.Extraction_Body:
		out.Source = &envoytransformation.Extraction_Body{
			Body: src.Body, // this is *empty.Empty but better to translate it now to avoid future confusion
		}
	case *transformation.Extraction_Header:
		out.Source = &envoytransformation.Extraction_Header{
			Header: src.Header,
		}
	}

	mode := extractor.GetMode()

	// if mode isn't in the list of extraction modes, set it to EXTRACT
	if _, ok := transformation.Extraction_Mode_name[int32(mode)]; !ok {
		mode = transformation.Extraction_EXTRACT
	}

	switch mode {
	case transformation.Extraction_EXTRACT:
		out.Mode = envoytransformation.Extraction_EXTRACT
		out.Subgroup = extractor.GetSubgroup()

		// error if replacement_text is set
		if extractor.GetReplacementText().GetValue() != "" {
			return nil, NewExtractorError(ErrMsgReplacementTextSetWhenNotNeeded, name, mode)
		}
	case transformation.Extraction_SINGLE_REPLACE:
		out.Mode = envoytransformation.Extraction_SINGLE_REPLACE
		out.Subgroup = extractor.GetSubgroup()
		out.ReplacementText = extractor.GetReplacementText()

		// error if replacement_text is not set
		if extractor.GetReplacementText() == nil {
			return nil, NewExtractorError(ErrMsgReplacementTextNotSetWhenNeeded, name, mode)
		}
	case transformation.Extraction_REPLACE_ALL:
		out.Mode = envoytransformation.Extraction_REPLACE_ALL
		out.ReplacementText = extractor.GetReplacementText()

		// error if subgroup is set
		if extractor.GetSubgroup() != 0 {
			return nil, NewExtractorError(ErrMsgSubgroupSetWhenNotNeeded, name, mode)
		}

		// error if replacement_text is not set
		if extractor.GetReplacementText() == nil {
			return nil, NewExtractorError(ErrMsgReplacementTextNotSetWhenNeeded, name, mode)
		}
	default:
		return nil, NewExtractorError("unknown extraction mode", name, mode)
	}

	return out, nil
}

func (p *Plugin) validateTransformation(
	ctx context.Context,
	transformations *envoytransformation.RouteTransformations,
) error {
	// If the user has disabled transformation validation, then always return nil
	if p.settings.GetGateway().GetValidation().GetDisableTransformationValidation().GetValue() {
		return nil
	}

	return p.validator.ValidateConfig(ctx, transformations)
}

func (p *Plugin) getTransformations(
	ctx context.Context,
	stage uint32,
	transformations *transformation.RequestResponseTransformations,
	stagedEscapeCharacters *wrapperspb.BoolValue,
) ([]*envoytransformation.RouteTransformations_RouteTransformation, error) {
	var outTransformations []*envoytransformation.RouteTransformations_RouteTransformation
	for _, t := range transformations.GetResponseTransforms() {
		responseTransform, err := p.TranslateTransformation(t.GetResponseTransformation(), p.escapeCharacters, stagedEscapeCharacters)
		if err != nil {
			return nil, err
		}
		outTransformations = append(outTransformations, &envoytransformation.RouteTransformations_RouteTransformation{
			Stage: stage,
			Match: &envoytransformation.RouteTransformations_RouteTransformation_ResponseMatch_{
				ResponseMatch: &envoytransformation.RouteTransformations_RouteTransformation_ResponseMatch{
					Match:                  getResponseMatcher(ctx, t),
					ResponseTransformation: responseTransform,
				},
			},
		})
	}

	for _, t := range transformations.GetRequestTransforms() {
		requestTransform, err := p.TranslateTransformation(t.GetRequestTransformation(), p.escapeCharacters, stagedEscapeCharacters)
		if err != nil {
			return nil, err
		}
		if t.GetResponseTransformation().GetTransformationTemplate().GetSpanTransformer() != nil {
			return nil, NewInvalidSpanTransformerError("spanTransformer cannot be set on a responseTransformation")
		}
		responseTransform, err := p.TranslateTransformation(t.GetResponseTransformation(), p.escapeCharacters, stagedEscapeCharacters)
		if err != nil {
			return nil, err
		}
		outTransformations = append(outTransformations, &envoytransformation.RouteTransformations_RouteTransformation{
			Stage: stage,
			Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
				RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
					Match:                  getRequestMatcher(ctx, t.GetMatcher()),
					RequestTransformation:  requestTransform,
					ClearRouteCache:        t.GetClearRouteCache(),
					ResponseTransformation: responseTransform,
				},
			},
		})
	}
	return outTransformations, nil
}

// Note: these are copied from the translator and adapted to v3 apis. Once the transformer
// is v3 ready, we can remove these.
func getResponseMatcher(ctx context.Context, m *transformation.ResponseMatch) *envoytransformation.ResponseMatcher {
	matcher := &envoytransformation.ResponseMatcher{
		Headers: envoyHeaderMatcher(ctx, m.GetMatchers()),
	}
	if m.GetResponseCodeDetails() != "" {
		matcher.ResponseCodeDetails = &v3.StringMatcher{
			MatchPattern: &v3.StringMatcher_Exact{Exact: m.GetResponseCodeDetails()},
		}
	}
	return matcher
}

func getRequestMatcher(ctx context.Context, matcher *matchers.Matcher) *envoyroutev3.RouteMatch {
	if matcher == nil {
		return nil
	}
	match := &envoyroutev3.RouteMatch{
		Headers:         envoyHeaderMatcher(ctx, matcher.GetHeaders()),
		QueryParameters: envoyQueryMatcher(ctx, matcher.GetQueryParameters()),
	}
	if len(matcher.GetMethods()) > 0 {
		match.Headers = append(match.GetHeaders(), &envoyroutev3.HeaderMatcher{
			Name: ":method",
			HeaderMatchSpecifier: &envoyroutev3.HeaderMatcher_SafeRegexMatch{
				SafeRegexMatch: convertRegex(regexutils.NewRegex(ctx, strings.Join(matcher.GetMethods(), "|"))),
			},
		})
	}
	// need to do this because Go's proto implementation makes oneofs private
	// which genius thought of that?
	setEnvoyPathMatcher(ctx, matcher, match)
	return match
}

func setEnvoyPathMatcher(ctx context.Context, in *matchers.Matcher, out *envoyroutev3.RouteMatch) {
	switch path := in.GetPathSpecifier().(type) {
	case *matchers.Matcher_Exact:
		out.PathSpecifier = &envoyroutev3.RouteMatch_Path{
			Path: path.Exact,
		}
	case *matchers.Matcher_Regex:
		out.PathSpecifier = &envoyroutev3.RouteMatch_SafeRegex{
			SafeRegex: convertRegex(regexutils.NewRegex(ctx, path.Regex)),
		}
	case *matchers.Matcher_Prefix:
		out.PathSpecifier = &envoyroutev3.RouteMatch_Prefix{
			Prefix: path.Prefix,
		}
	case *matchers.Matcher_ConnectMatcher_:
		out.PathSpecifier = &envoyroutev3.RouteMatch_ConnectMatcher_{
			ConnectMatcher: &envoyroutev3.RouteMatch_ConnectMatcher{},
		}
	}
}

func envoyQueryMatcher(
	ctx context.Context,
	in []*matchers.QueryParameterMatcher,
) []*envoyroutev3.QueryParameterMatcher {
	var out []*envoyroutev3.QueryParameterMatcher
	for _, matcher := range in {
		envoyMatch := &envoyroutev3.QueryParameterMatcher{
			Name: matcher.GetName(),
		}

		if matcher.GetValue() == "" {
			envoyMatch.QueryParameterMatchSpecifier = &envoyroutev3.QueryParameterMatcher_PresentMatch{
				PresentMatch: true,
			}
		} else {
			if matcher.GetRegex() {
				envoyMatch.QueryParameterMatchSpecifier = &envoyroutev3.QueryParameterMatcher_StringMatch{
					StringMatch: &v3.StringMatcher{
						MatchPattern: &v3.StringMatcher_SafeRegex{
							SafeRegex: convertRegex(regexutils.NewRegex(ctx, matcher.GetValue())),
						},
					},
				}
			} else {
				envoyMatch.QueryParameterMatchSpecifier = &envoyroutev3.QueryParameterMatcher_StringMatch{
					StringMatch: &v3.StringMatcher{
						MatchPattern: &v3.StringMatcher_Exact{
							Exact: matcher.GetValue(),
						},
					},
				}
			}
		}
		out = append(out, envoyMatch)
	}
	return out
}

func envoyHeaderMatcher(ctx context.Context, in []*matchers.HeaderMatcher) []*envoyroutev3.HeaderMatcher {
	var out []*envoyroutev3.HeaderMatcher
	for _, matcher := range in {
		envoyMatch := &envoyroutev3.HeaderMatcher{
			Name: matcher.GetName(),
		}
		if matcher.GetValue() == "" {
			envoyMatch.HeaderMatchSpecifier = &envoyroutev3.HeaderMatcher_PresentMatch{
				PresentMatch: true,
			}
		} else {
			if matcher.GetRegex() {
				regex := regexutils.NewRegex(ctx, matcher.GetValue())
				envoyMatch.HeaderMatchSpecifier = &envoyroutev3.HeaderMatcher_SafeRegexMatch{
					SafeRegexMatch: convertRegex(regex),
				}
			} else {
				envoyMatch.HeaderMatchSpecifier = &envoyroutev3.HeaderMatcher_ExactMatch{
					ExactMatch: matcher.GetValue(),
				}
			}
		}

		if matcher.GetInvertMatch() {
			envoyMatch.InvertMatch = true
		}
		out = append(out, envoyMatch)
	}
	return out
}

func convertRegex(regex *envoy_type_matcher_v3.RegexMatcher) *v3.RegexMatcher {
	if regex == nil {
		return nil
	}
	return &v3.RegexMatcher{
		EngineType: &v3.RegexMatcher_GoogleRe2{GoogleRe2: &v3.RegexMatcher_GoogleRE2{MaxProgramSize: regex.GetGoogleRe2().GetMaxProgramSize()}},
		Regex:      regex.GetRegex(),
	}
}
