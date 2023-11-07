package urlrewrite

import (
	"context"

	errors "github.com/rotisserie/eris"
	matcherv3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"google.golang.org/protobuf/types/known/wrapperspb"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ApplyFilter(
	ctx context.Context,
	filter gwv1.HTTPRouteFilter,
	outputRoute *v1.Route,
) error {
	config := filter.URLRewrite
	if config == nil {
		return errors.Errorf("UrlRewrite filter supplied does not define urlRewrite config")
	}

	if config.Hostname != nil {
		outputRoute.Options.HostRewriteType = &v1.RouteOptions_HostRewrite{
			HostRewrite: string(*config.Hostname),
		}
	}

	if config.Path != nil {
		switch config.Path.Type {
		case gwv1.FullPathHTTPPathModifier:
			if config.Path.ReplaceFullPath == nil {
				return errors.Errorf("UrlRewrite filter supplied with Full Path rewrite type, but no Full Path supplied")
			}
			outputRoute.Options.RegexRewrite = &matcherv3.RegexMatchAndSubstitute{
				Pattern: &matcherv3.RegexMatcher{
					Regex: ".*",
				},
				Substitution: *config.Path.ReplaceFullPath,
			}
		case gwv1.PrefixMatchHTTPPathModifier:
			if config.Path.ReplacePrefixMatch == nil {
				return errors.Errorf("UrlRewrite filter supplied with prefix rewrite type, but no prefix supplied")
			}
			outputRoute.Options.PrefixRewrite = &wrapperspb.StringValue{
				Value: *config.Path.ReplacePrefixMatch,
			}
		}
	}

	return nil
}
