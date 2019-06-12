package rest

/*
if this destination spec has rest service spec
this will grab the parameters from the route extension
*/
import (
	"context"
	"regexp"
	"strings"

	"github.com/gogo/protobuf/proto"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/errors"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"

	glooplugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins"
	transformapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

type UpstreamWithServiceSpec interface {
	GetServiceSpec() *glooplugins.ServiceSpec
}

type plugin struct {
	transformsAdded   *bool
	recordedUpstreams map[core.ResourceRef]*glooplugins.ServiceSpec_Rest
	ctx               context.Context
}

func NewPlugin(transformsAdded *bool) plugins.Plugin {
	return &plugin{transformsAdded: transformsAdded}
}

func (p *plugin) Init(params plugins.InitParams) error {
	p.ctx = params.Ctx
	p.recordedUpstreams = make(map[core.ResourceRef]*glooplugins.ServiceSpec_Rest)
	return nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, _ *envoyapi.Cluster) error {
	if withServiceSpec, ok := in.UpstreamSpec.UpstreamType.(UpstreamWithServiceSpec); ok {
		serviceSpec := withServiceSpec.GetServiceSpec()
		if serviceSpec == nil {
			return nil
		}

		if serviceSpec.PluginType == nil {
			return nil
		}

		restServiceSpec, ok := serviceSpec.PluginType.(*glooplugins.ServiceSpec_Rest)
		if !ok {
			return nil
		}
		if restServiceSpec.Rest == nil {
			return errors.Errorf("%v has an empty rest service spec", in.Metadata.Ref())
		}
		p.recordedUpstreams[in.Metadata.Ref()] = restServiceSpec
	}
	return nil
}

func (p *plugin) ProcessRoute(params plugins.Params, in *v1.Route, out *envoyroute.Route) error {
	return pluginutils.MarkPerFilterConfig(p.ctx, params.Snapshot, in, out, transformation.FilterName, func(spec *v1.Destination) (proto.Message, error) {
		// check if it's rest destination
		if spec.DestinationSpec == nil || spec.GetUpstream() == nil {
			return nil, nil
		}
		restDestinationSpec, ok := spec.DestinationSpec.DestinationType.(*v1.DestinationSpec_Rest)
		if !ok {
			return nil, nil
		}
		// get upstream
		restServiceSpec, ok := p.recordedUpstreams[*spec.GetUpstream()]
		if !ok {
			return nil, errors.Errorf("%v does not have a rest service spec", *spec.GetUpstream())
		}
		funcname := restDestinationSpec.Rest.FunctionName
		transformationorig := restServiceSpec.Rest.Transformations[funcname]
		if transformationorig == nil {
			return nil, errors.Errorf("unknown function %v", funcname)
		}

		// copy to prevent changing the original in memory.
		transformation := *transformationorig

		// add extensions from the destination spec
		var err error
		transformation.Extractors, err = p.createRequestExtractors(restDestinationSpec.Rest.Parameters)
		if err != nil {
			return nil, err
		}
		// should be aws upstream

		*p.transformsAdded = true

		// get function
		ret := &transformapi.RouteTransformations{
			RequestTransformation: &transformapi.Transformation{
				TransformationType: &transformapi.Transformation_TransformationTemplate{
					TransformationTemplate: &transformation,
				},
			},
		}

		*p.transformsAdded = true
		if restDestinationSpec.Rest.ResponseTransformation != nil {
			// TODO(yuval-k): should we add \ support response parameters?
			ret.ResponseTransformation = &transformapi.Transformation{
				TransformationType: &transformapi.Transformation_TransformationTemplate{
					TransformationTemplate: restDestinationSpec.Rest.ResponseTransformation,
				},
			}
		}

		return ret, nil
	})
}

func (p *plugin) createRequestExtractors(params *transformapi.Parameters) (map[string]*transformapi.Extraction, error) {
	extractors := make(map[string]*transformapi.Extraction)
	if params == nil {
		return extractors, nil
	}

	// special http2 headers, get the whole thing for free
	// as a convenience to the user
	// TODO: add more
	for _, header := range []string{
		"path",
		"method",
	} {
		p.addHeaderExtractorFromParam(":"+header, "{"+header+"}", extractors)
	}
	// headers we support submatching on
	// custom as well as the path and authority/host header
	if params.Path != nil {
		if err := p.addHeaderExtractorFromParam(":path", params.Path.Value, extractors); err != nil {
			return nil, errors.Wrapf(err, "error processing parameter")
		}
	}
	for headerName, headerValue := range params.Headers {
		if err := p.addHeaderExtractorFromParam(headerName, headerValue, extractors); err != nil {
			return nil, errors.Wrapf(err, "error processing parameter")
		}
	}
	return extractors, nil
}

func (p *plugin) addHeaderExtractorFromParam(header, parameter string, extractors map[string]*transformapi.Extraction) error {
	if parameter == "" {
		return nil
	}
	// remember that the order of the param names correlates with their order in the regex
	paramNames, regexMatcher := getNamesAndRegexFromParamString(parameter)
	contextutils.LoggerFrom(p.ctx).Debugf("transformation pluginN: extraction for header %v: parameters: %v regex matcher: %v", header, paramNames, regexMatcher)
	// if no regex, this is a "default variable" that the user gets for free
	if len(paramNames) == 0 {
		// extract everything
		// TODO(yuval): create a special extractor that doesn't use regex when we just want the whole thing
		extract := &transformapi.Extraction{
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
		extract := &transformapi.Extraction{
			Header:   header,
			Regex:    regexMatcher,
			Subgroup: uint32(i + 1),
		}
		extractors[name] = extract
	}
	return nil
}

var rxp = regexp.MustCompile(`\{([\.\-_[:word:]]+)\}`)

func getNamesAndRegexFromParamString(paramString string) ([]string, string) {
	// escape regex
	// TODO: make sure all envoy regex is being escaped here
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
