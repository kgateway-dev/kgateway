package rest

/*
if this destination spec has rest service spec
this will grab the parameters from the route extension
*/
import (
	"context"
	"regexp"
	"strings"

	"github.com/solo-io/solo-kit/pkg/errors"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	transformapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/go-utils/contextutils"
)

func CreateRequestExtractors(ctx context.Context, params *transformapi.Parameters) (map[string]*transformation.Extraction, error) {
	extractors := make(map[string]*transformation.Extraction)
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
		addHeaderExtractorFromParam(ctx, ":"+header, "{"+header+"}", extractors)
	}
	// headers we support submatching on
	// custom as well as the path and authority/host header
	if params.GetPath() != nil {
		if err := addHeaderExtractorFromParam(ctx, ":path", params.GetPath().GetValue(), extractors); err != nil {
			return nil, errors.Wrapf(err, "error processing parameter")
		}
	}
	for headerName, headerValue := range params.GetHeaders() {
		if err := addHeaderExtractorFromParam(ctx, headerName, headerValue, extractors); err != nil {
			return nil, errors.Wrapf(err, "error processing parameter")
		}
	}
	return extractors, nil
}

func addHeaderExtractorFromParam(ctx context.Context, header, parameter string, extractors map[string]*transformation.Extraction) error {
	if parameter == "" {
		return nil
	}
	// remember that the order of the param names correlates with their order in the regex
	paramNames, regexMatcher := getNamesAndRegexFromParamString(parameter)
	contextutils.LoggerFrom(ctx).Debugf("transformation pluginN: extraction for header %v: parameters: %v regex matcher: %v", header, paramNames, regexMatcher)
	// if no regex, this is a "default variable" that the user gets for free
	if len(paramNames) == 0 {
		// extract everything
		// TODO(yuval): create a special extractor that doesn't use regex when we just want the whole thing
		extract := &transformation.Extraction{
			Source:   &transformation.Extraction_Header{Header: header},
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
		extract := &transformation.Extraction{
			Source:   &transformation.Extraction_Header{Header: header},
			Regex:    regexMatcher,
			Subgroup: uint32(i + 1),
		}
		extractors[name] = extract
	}
	return nil
}

var rxp = regexp.MustCompile(`\{\s*([\.\-_[:word:]]+)\s*\}`)

func getNamesAndRegexFromParamString(paramString string) ([]string, string) {
	// escape regex
	// TODO: make sure all envoy regex is being escaped here
	parameterNames := rxp.FindAllString(paramString, -1)
	for i, name := range parameterNames {
		parameterNames[i] = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(name, "{"), "}"))
	}

	return parameterNames, buildRegexString(rxp, paramString)
}

func buildRegexString(rxp *regexp.Regexp, paramString string) string {
	var regexString string
	var prevEnd int
	for _, startStop := range rxp.FindAllStringIndex(paramString, -1) {
		start := startStop[0]
		end := startStop[1]
		subStr := regexp.QuoteMeta(paramString[prevEnd:start]) + `([\-._%[:alnum:]]+)`
		regexString += subStr
		prevEnd = end
	}

	return regexString + regexp.QuoteMeta(paramString[prevEnd:])
}
