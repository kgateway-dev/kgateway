package transformation

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"

	"fmt"
	"regexp"
	"strings"

	"github.com/gogo/protobuf/types"
	"github.com/mitchellh/hashstructure"
	"github.com/pkg/errors"

	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/common"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
)

//go:generate protoc -I=./envoy/ -I=${GOPATH}/src/github.com/gogo/protobuf/protobuf/ --gogo_out=. envoy/transformation_filter.proto

const (
	filterName          = "io.solo.transformation"
	metadataRequestKey  = "request-transformation"
	metadataResponseKey = "response-transformation"

	ServiceTypeTransformation = "HTTP-Functions"
	pluginStage               = plugins.PostInAuth
)

type GetTransformationFunction func(destination *v1.Destination_Function) (*TransformationTemplate, error)

type Plugin interface {
	ActivateFilterForCluster(out *envoyapi.Cluster)
	AddRequestTransformationsToRoute(getTemplate GetTransformationFunction, in *v1.Route, out *envoyroute.Route) error
	AddResponseTransformationsToRoute(in *v1.Route, out *envoyroute.Route) error
	GetTransformationFilter() *plugins.StagedHttpFilter
}

func NewTransformationPlugin() Plugin {
	return &transformationPlugin{
		cachedTransformations: make(map[string]*Transformation),
	}
}

type transformationPlugin struct {
	cachedTransformations map[string]*Transformation
}

func (p *transformationPlugin) ActivateFilterForCluster(out *envoyapi.Cluster) {
	if out.Metadata == nil {
		out.Metadata = &envoycore.Metadata{}
	}
	common.InitFilterMetadata(filterName, out.Metadata)
	out.Metadata.FilterMetadata[filterName] = &types.Struct{
		Fields: make(map[string]*types.Value),
	}
}

func (p *transformationPlugin) AddRequestTransformationsToRoute(getTemplate GetTransformationFunction, in *v1.Route, out *envoyroute.Route) error {
	var extractors map[string]*Extraction
	// if no parameters specified, the only extraction will be a json body
	if in.Extensions != nil {
		extension, err := DecodeRouteExtension(in.Extensions)
		if err != nil {
			return err
		}
		extractors, err = createRequestExtractors(extension.Parameters)
		if err != nil {
			return err
		}
	}

	// calculate the templates for all these transformations
	if err := p.setTransformationsForRoute(getTemplate, in, extractors, out); err != nil {
		return errors.Wrap(err, "resolving request transformations for route")
	}

	return nil
}

func createRequestExtractors(params *Parameters) (map[string]*Extraction, error) {
	extractors := make(map[string]*Extraction)
	if params == nil {
		return extractors, nil
	}

	// special http2 headers, get the whole thing for free
	// as a convenience to the user
	// TODO: add more
	for _, header := range []string{
		"path",
		"method",
		"scheme",
		"authority",
	} {
		addHeaderExtractorFromParam(":"+header, "{"+header+"}", extractors)
	}
	// headers we support submatching on
	// custom as well as the path and authority/host header
	if params.Path != nil {
		if err := addHeaderExtractorFromParam(":path", *params.Path, extractors); err != nil {
			return nil, errors.Wrap(err, "error processing parameter")
		}
	}
	if params.Authority != nil {
		if err := addHeaderExtractorFromParam(":authority", *params.Authority, extractors); err != nil {
			return nil, errors.Wrap(err, "error processing parameter")
		}
	}
	for headerName, headerValue := range params.Headers {
		if err := addHeaderExtractorFromParam(headerName, headerValue, extractors); err != nil {
			return nil, errors.Wrap(err, "error processing parameter")
		}
	}
	return extractors, nil
}

// TODO: clean up the response transformation
// params should live on the source (upstream/function)
func (p *transformationPlugin) AddResponseTransformationsToRoute(in *v1.Route, out *envoyroute.Route) error {
	if in.Extensions == nil {
		return nil
	}

	extension, err := DecodeRouteExtension(in.Extensions)
	if err != nil {
		return err
	}

	if extension.ResponseTemplate == nil {
		return nil
	}

	extractors := make(map[string]*Extraction)

	if extension.ResponseParams != nil {
		for headerName, headerValue := range extension.ResponseParams.Headers {
			addHeaderExtractorFromParam(headerName, headerValue, extractors)
		}
	}

	// calculate the templates for all these transformations
	if err := p.setResponseTransformationForRoute(*extension.ResponseTemplate, extractors, out); err != nil {
		return errors.Wrap(err, "resolving request transformations for route")
	}

	return nil
}

func addHeaderExtractorFromParam(header, parameter string, extractors map[string]*Extraction) error {
	if parameter == "" {
		return nil
	}
	// remember that the order of the param names correlates with their order in the regex
	paramNames, regexMatcher := getNamesAndRegexFromParamString(parameter)
	log.Debugf("transformation pluginN: extraction for header %v: parameters: %v regex matcher: %v", header, paramNames, regexMatcher)
	// if no regex, this is a "default variable" that the user gets for free
	if len(paramNames) == 0 {
		// extract everything
		// TODO(yuval): create a special extractor that doesn't use regex when we just want the whole thing
		extract := &Extraction{
			Header:   header,
			Regex:    "(.*)",
			Subgroup: uint32(1),
		}
		extractors[strings.TrimPrefix(header, ":")] = extract
	}

	// count the number of open braces,
	// if they are not equal to the # of counted params,
	// the user gave us bad variable names or unterminated braces and we should error
	expectedParameterCount := strings.Count(parameter, "{")
	if len(paramNames) != expectedParameterCount {
		return errors.Errorf("%v is not valid syntax. {} braces must be closed and variable names must satisfy regex "+
			`([\-._[:alnum:]]+)`, parameter)
	}

	// otherwise it's regex, and we need to create an extraction for each variable name they defined
	for i, name := range paramNames {
		extract := &Extraction{
			Header:   header,
			Regex:    regexMatcher,
			Subgroup: uint32(i + 1),
		}
		extractors[name] = extract
	}
	return nil
}

func getNamesAndRegexFromParamString(paramString string) ([]string, string) {
	// escape regex
	// TODO: make sure all envoy regex is being escaped here
	rxp := regexp.MustCompile(`\{([\.\-_[:word:]]+)\}`)
	parameterNames := rxp.FindAllString(paramString, -1)
	for i, name := range parameterNames {
		parameterNames[i] = strings.TrimSuffix(strings.TrimPrefix(name, "{"), "}")
	}

	return parameterNames, buildRegexString(rxp, paramString)
}

func buildRegexString(rxp *regexp.Regexp, paramString string) string {
	var regexString string
	var prevEnd int
	for _, startStop := range rxp.FindAllStringIndex(paramString, -1) {
		start := startStop[0]
		end := startStop[1]
		subStr := regexp.QuoteMeta(paramString[prevEnd:start]) + `([\-._[:alnum:]]+)`
		regexString += subStr
		prevEnd = end
	}

	return regexString + regexp.QuoteMeta(paramString[prevEnd:])
}

// sets all transformations a route may need
// if single destination, just one transformation
// if multi destination, one transformation for each functional
// that specifies a transformation spec
func (p *transformationPlugin) setTransformationsForRoute(getTemplate GetTransformationFunction, in *v1.Route, extractors map[string]*Extraction, out *envoyroute.Route) error {
	switch {
	case in.MultipleDestinations != nil:
		for _, dest := range in.MultipleDestinations {
			err := p.setTransformationForRoute(getTemplate, dest.Destination, extractors, out)
			if err != nil {
				return errors.Wrap(err, "setting transformation for route")
			}
		}
	case in.SingleDestination != nil:
		err := p.setTransformationForRoute(getTemplate, in.SingleDestination, extractors, out)
		if err != nil {
			return errors.Wrap(err, "setting transformation for route")
		}
	}
	return nil
}

func (p *transformationPlugin) setTransformationForRoute(getTemplateForDestination GetTransformationFunction, dest *v1.Destination, extractors map[string]*Extraction, out *envoyroute.Route) error {
	fnDestination, ok := dest.DestinationType.(*v1.Destination_Function)
	if !ok {
		// not a functional route, nothing to do
		return nil
	}
	template, err := getTemplateForDestination(fnDestination)
	if err != nil {
		return errors.Wrap(err, "getting transformation for function")
	}
	// no transformations for this destination
	if template == nil {
		return nil
	}
	template.Extractors = extractors

	t := Transformation{
		TransformationType: &Transformation_TransformationTemplate{
			TransformationTemplate: template,
		},
	}

	intHash, err := hashstructure.Hash(t, nil)
	if err != nil {
		return err
	}

	hash := fmt.Sprintf("%v", intHash)

	// cache the transformation, the filter config needs to contain all of them
	p.cachedTransformations[hash] = &t

	// set the filter metadata on the route
	if out.Metadata == nil {
		out.Metadata = &envoycore.Metadata{}
	}
	filterMetadata := common.InitFilterMetadataField(filterName, metadataRequestKey, out.Metadata)
	if filterMetadata.Kind == nil {
		filterMetadata.Kind = &types.Value_StructValue{}
	}
	if _, ok := filterMetadata.Kind.(*types.Value_StructValue); !ok {
		return errors.Errorf("needed filter metadta to be kind *types.Value_StructValue, but was: %v", filterMetadata.Kind)
	}
	if filterMetadata.Kind.(*types.Value_StructValue).StructValue == nil {
		filterMetadata.Kind.(*types.Value_StructValue).StructValue = &types.Struct{}
	}
	if filterMetadata.Kind.(*types.Value_StructValue).StructValue.Fields == nil {
		filterMetadata.Kind.(*types.Value_StructValue).StructValue.Fields = make(map[string]*types.Value)
	}

	upstreamName := fnDestination.Function.UpstreamName
	functionName := fnDestination.Function.FunctionName

	fields := filterMetadata.Kind.(*types.Value_StructValue).StructValue.Fields
	if fields[upstreamName] == nil {
		var funcVal types.Value
		funcVal.Kind = &types.Value_StructValue{
			StructValue: &types.Struct{
				Fields: make(map[string]*types.Value),
			},
		}
		fields[upstreamName] = &funcVal
	}

	funcFields := fields[upstreamName].Kind.(*types.Value_StructValue).StructValue.Fields
	if funcFields[functionName] == nil {
		funcFields[functionName] = &types.Value{
			Kind: &types.Value_StructValue{
				StructValue: &types.Struct{
					Fields: make(map[string]*types.Value),
				},
			},
		}
	}
	funcFields[functionName].Kind = &types.Value_StringValue{StringValue: hash}

	return nil
}

func (p *transformationPlugin) setResponseTransformationForRoute(template Template, extractors map[string]*Extraction, out *envoyroute.Route) error {
	// create templates
	// right now it's just a no-op, user writes inja directly
	headerTemplates := make(map[string]*InjaTemplate)
	for k, v := range template.Header {
		headerTemplates[k] = &InjaTemplate{Text: v}
	}

	tt := &Transformation_TransformationTemplate{
		TransformationTemplate: &TransformationTemplate{
			Extractors: extractors,
			Headers:    headerTemplates,
		},
	}

	if template.Body != nil {
		tt.TransformationTemplate.BodyTransformation = &TransformationTemplate_Body{
			Body: &InjaTemplate{
				Text: *template.Body,
			},
		}
	} else {
		tt.TransformationTemplate.BodyTransformation = &TransformationTemplate_Passthrough{
			Passthrough: &Passthrough{},
		}
	}

	t := Transformation{
		TransformationType: tt,
	}

	intHash, err := hashstructure.Hash(t, nil)
	if err != nil {
		return errors.Wrap(err, "generating hash")
	}

	hash := fmt.Sprintf("%v", intHash)

	// cache the transformation, the filter config needs to contain all of them
	p.cachedTransformations[hash] = &t

	// set the filter metadata on the route
	if out.Metadata == nil {
		out.Metadata = &envoycore.Metadata{}
	}
	filterMetadata := common.InitFilterMetadataField(filterName, metadataResponseKey, out.Metadata)
	filterMetadata.Kind = &types.Value_StringValue{StringValue: hash}

	return nil
}

func (p *transformationPlugin) GetTransformationFilter() *plugins.StagedHttpFilter {
	if len(p.cachedTransformations) == 0 {
		return nil
	}
	defer func() {
		// clear cache
		p.cachedTransformations = make(map[string]*Transformation)
	}()

	filterConfig, err := util.MessageToStruct(&Transformations{
		Transformations: p.cachedTransformations,
	})
	if err != nil {
		log.Warnf("error in transformation plugin: %v", err)
		return nil
	}

	return &plugins.StagedHttpFilter{
		HttpFilter: &envoyhttp.HttpFilter{
			Name:   filterName,
			Config: filterConfig,
		}, Stage: pluginStage,
	}
}
