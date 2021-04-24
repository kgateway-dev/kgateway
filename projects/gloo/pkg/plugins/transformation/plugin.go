package transformation

import (
	"context"
	"fmt"
	"strings"

	"github.com/rotisserie/eris"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/solo-io/gloo/pkg/utils/regexutils"
	envoyroutev3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/route/v3"
	envoytransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

const (
	FilterName       = "io.solo.transformation"
	EarlyStageNumber = 1

	PluginName = "transformation.plugin.solo"
)

var (
	earlyPluginStage = plugins.AfterStage(plugins.FaultStage)
	pluginStage      = plugins.AfterStage(plugins.AuthZStage)
)

var (
	UnknownTransformationType = func(transformation interface{}) error {
		return fmt.Errorf("unknown transformation type %T", transformation)
	}
)

var _ plugins.Plugin = new(Plugin)

var _ plugins.VirtualHostPlugin = new(Plugin)
var _ plugins.WeightedDestinationPlugin = new(Plugin)
var _ plugins.RoutePlugin = new(Plugin)
var _ plugins.HttpFilterPlugin = new(Plugin)

type TranslateTransformationFn func(*transformation.Transformation) (*envoytransformation.Transformation, error)

type Plugin struct {
	RequireTransformationFilter bool
	requireEarlyTransformation  bool
	TranslateTransformation     TranslateTransformationFn
	settings                    *v1.Settings
}

func (p *Plugin) PluginName() string {
	return PluginName
}

func (p *Plugin) IsUpgrade() bool {
	return false
}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	p.RequireTransformationFilter = false
	p.requireEarlyTransformation = false
	p.settings = params.Settings
	p.TranslateTransformation = TranslateTransformation
	return nil
}

func (p *Plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {
	envoyTransformation, err := p.convertTransformation(
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
	p.RequireTransformationFilter = true
	err = p.validateTransformation(params.Ctx, envoyTransformation)
	if err != nil {
		return err
	}

	p.RequireTransformationFilter = true
	return pluginutils.SetVhostPerFilterConfig(out, FilterName, envoyTransformation)
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	envoyTransformation, err := p.convertTransformation(
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
	p.RequireTransformationFilter = true
	err = p.validateTransformation(params.Ctx, envoyTransformation)
	if err != nil {
		return err
	}

	p.RequireTransformationFilter = true
	return pluginutils.SetRoutePerFilterConfig(out, FilterName, envoyTransformation)
}

func (p *Plugin) ProcessWeightedDestination(
	params plugins.RouteParams,
	in *v1.WeightedDestination,
	out *envoy_config_route_v3.WeightedCluster_ClusterWeight,
) error {
	envoyTransformation, err := p.convertTransformation(
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

	p.RequireTransformationFilter = true
	err = p.validateTransformation(params.Ctx, envoyTransformation)
	if err != nil {
		return err
	}

	return pluginutils.SetWeightedClusterPerFilterConfig(out, FilterName, envoyTransformation)
}

func (p *Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	earlyStageConfig := &envoytransformation.FilterTransformations{
		Stage: EarlyStageNumber,
	}
	earlyFilter, err := plugins.NewStagedFilterWithConfig(FilterName, earlyStageConfig, earlyPluginStage)
	if err != nil {
		return nil, err
	}
	var filters []plugins.StagedHttpFilter
	if p.requireEarlyTransformation {
		// only add early transformations if we have to, to allow rolling gloo updates;
		// i.e. an older envoy without stages connects to gloo, it shouldn't have 2 filters.
		filters = append(filters, earlyFilter)
	}
	filters = append(filters, plugins.NewStagedFilter(FilterName, pluginStage))
	return filters, nil
}

func (p *Plugin) convertTransformation(
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
		requestTransform, err := p.TranslateTransformation(t.RequestTransformation)
		if err != nil {
			return nil, err
		}
		responseTransform, err := p.TranslateTransformation(t.ResponseTransformation)
		if err != nil {
			return nil, err
		}
		ret.RequestTransformation = requestTransform
		ret.ClearRouteCache = t.ClearRouteCache
		ret.ResponseTransformation = responseTransform
		// new config:
		// we have to have it too, as if any new config is defined the deprecated config is ignored.
		ret.Transformations = append(ret.Transformations, &envoytransformation.RouteTransformations_RouteTransformation{
			Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
				RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
					Match:                  nil,
					RequestTransformation:  requestTransform,
					ClearRouteCache:        t.ClearRouteCache,
					ResponseTransformation: responseTransform,
				},
			},
		})
	}

	if early := stagedTransformations.GetEarly(); early != nil {
		p.requireEarlyTransformation = true
		transformations, err := p.getTransformations(ctx, EarlyStageNumber, early)
		if err != nil {
			return nil, err
		}
		ret.Transformations = append(ret.Transformations, transformations...)
	}
	if regular := stagedTransformations.GetRegular(); regular != nil {
		transformations, err := p.getTransformations(ctx, 0, regular)
		if err != nil {
			return nil, err
		}
		ret.Transformations = append(ret.Transformations, transformations...)
	}
	return ret, nil
}

func (p *Plugin) translateOSSTransformations(glooTransform *transformation.Transformation) (*envoytransformation.Transformation, error) {
	transform, err := p.TranslateTransformation(glooTransform)
	if err != nil {
		return nil, eris.Wrap(err, "this transformation type is not supported in open source Gloo Edge")
	}
	return transform, nil
}

func TranslateTransformation(glooTransform *transformation.Transformation) (*envoytransformation.Transformation, error) {
	if glooTransform == nil {
		return nil, nil
	}
	out := &envoytransformation.Transformation{}

	switch typedTransformation := glooTransform.GetTransformationType().(type) {
	case *transformation.Transformation_HeaderBodyTransform:
		{
			out.TransformationType = &envoytransformation.Transformation_HeaderBodyTransform{
				HeaderBodyTransform: typedTransformation.HeaderBodyTransform,
			}
		}
	case *transformation.Transformation_TransformationTemplate:
		{
			out.TransformationType = &envoytransformation.Transformation_TransformationTemplate{
				TransformationTemplate: typedTransformation.TransformationTemplate,
			}
		}
	default:
		return nil, UnknownTransformationType(typedTransformation)
	}
	return out, nil
}

func (p *Plugin) validateTransformation(ctx context.Context, transformations *envoytransformation.RouteTransformations) error {
	err := bootstrap.ValidateBootstrap(ctx, p.settings, FilterName, transformations)
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) getTransformations(ctx context.Context, stage uint32, transformations *transformation.RequestResponseTransformations) ([]*envoytransformation.RouteTransformations_RouteTransformation, error) {
	var outTransformations []*envoytransformation.RouteTransformations_RouteTransformation
	for _, transformation := range transformations.GetResponseTransforms() {
		responseTransform, err := p.TranslateTransformation(transformation.ResponseTransformation)
		if err != nil {
			return nil, err
		}
		outTransformations = append(outTransformations, &envoytransformation.RouteTransformations_RouteTransformation{
			Stage: stage,
			Match: &envoytransformation.RouteTransformations_RouteTransformation_ResponseMatch_{
				ResponseMatch: &envoytransformation.RouteTransformations_RouteTransformation_ResponseMatch{
					Match:                  getResponseMatcher(ctx, transformation),
					ResponseTransformation: responseTransform,
				},
			},
		})
	}

	for _, transformation := range transformations.GetRequestTransforms() {
		requestTransform, err := p.TranslateTransformation(transformation.RequestTransformation)
		if err != nil {
			return nil, err
		}
		responseTransform, err := p.TranslateTransformation(transformation.ResponseTransformation)
		if err != nil {
			return nil, err
		}
		outTransformations = append(outTransformations, &envoytransformation.RouteTransformations_RouteTransformation{
			Stage: stage,
			Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
				RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
					Match:                  getRequestMatcher(ctx, transformation.GetMatcher()),
					RequestTransformation:  requestTransform,
					ClearRouteCache:        transformation.ClearRouteCache,
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
	if m.ResponseCodeDetails != "" {
		matcher.ResponseCodeDetails = &v3.StringMatcher{
			MatchPattern: &v3.StringMatcher_Exact{Exact: m.ResponseCodeDetails},
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
		match.Headers = append(match.Headers, &envoyroutev3.HeaderMatcher{
			Name: ":method",
			HeaderMatchSpecifier: &envoyroutev3.HeaderMatcher_SafeRegexMatch{
				SafeRegexMatch: convertRegex(regexutils.NewRegex(ctx, strings.Join(matcher.Methods, "|"))),
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
	}
}

func envoyQueryMatcher(ctx context.Context, in []*matchers.QueryParameterMatcher) []*envoyroutev3.QueryParameterMatcher {
	var out []*envoyroutev3.QueryParameterMatcher
	for _, matcher := range in {
		envoyMatch := &envoyroutev3.QueryParameterMatcher{
			Name: matcher.Name,
		}

		if matcher.Value == "" {
			envoyMatch.QueryParameterMatchSpecifier = &envoyroutev3.QueryParameterMatcher_PresentMatch{
				PresentMatch: true,
			}
		} else {
			if matcher.Regex {
				envoyMatch.QueryParameterMatchSpecifier = &envoyroutev3.QueryParameterMatcher_StringMatch{
					StringMatch: &v3.StringMatcher{
						MatchPattern: &v3.StringMatcher_SafeRegex{
							SafeRegex: convertRegex(regexutils.NewRegex(ctx, matcher.Value)),
						},
					},
				}
			} else {
				envoyMatch.QueryParameterMatchSpecifier = &envoyroutev3.QueryParameterMatcher_StringMatch{
					StringMatch: &v3.StringMatcher{
						MatchPattern: &v3.StringMatcher_Exact{
							Exact: matcher.Value,
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
			Name: matcher.Name,
		}
		if matcher.Value == "" {
			envoyMatch.HeaderMatchSpecifier = &envoyroutev3.HeaderMatcher_PresentMatch{
				PresentMatch: true,
			}
		} else {
			if matcher.Regex {
				regex := regexutils.NewRegex(ctx, matcher.Value)
				envoyMatch.HeaderMatchSpecifier = &envoyroutev3.HeaderMatcher_SafeRegexMatch{
					SafeRegexMatch: convertRegex(regex),
				}
			} else {
				envoyMatch.HeaderMatchSpecifier = &envoyroutev3.HeaderMatcher_ExactMatch{
					ExactMatch: matcher.Value,
				}
			}
		}

		if matcher.InvertMatch {
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
