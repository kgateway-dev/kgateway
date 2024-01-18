package urlrewrite

import (
	"context"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	matcherv3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"google.golang.org/protobuf/types/known/wrapperspb"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ plugins.RoutePlugin = &plugin{}

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) ApplyRoutePlugin(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
	outputRoute *v1.Route,
) error {
	filter := utils.FindAppliedRouteFilter(routeCtx, gwv1.HTTPRouteFilterURLRewrite)
	if filter == nil {
		return nil
	}

	config := filter.URLRewrite
	if config == nil {
		return errors.Errorf("UrlRewrite filter supplied does not define urlRewrite config")
	}

	if config.Hostname != nil {
		outputRoute.GetOptions().HostRewriteType = &v1.RouteOptions_HostRewrite{
			HostRewrite: string(*config.Hostname),
		}
	}

	if config.Path != nil {
		switch config.Path.Type {
		case gwv1.FullPathHTTPPathModifier:
			if config.Path.ReplaceFullPath == nil {
				return errors.Errorf("UrlRewrite filter supplied with Full Path rewrite type, but no Full Path supplied")
			}
			outputRoute.GetOptions().RegexRewrite = &matcherv3.RegexMatchAndSubstitute{
				Pattern: &matcherv3.RegexMatcher{
					Regex: ".*",
				},
				Substitution: *config.Path.ReplaceFullPath,
			}
		case gwv1.PrefixMatchHTTPPathModifier:
			if config.Path.ReplacePrefixMatch == nil {
				return errors.Errorf("UrlRewrite filter supplied with prefix rewrite type, but no prefix supplied")
			}
			// Circumvent the case of "//" when the replace string is "/"
			// An empty replace string does not seem to solve the issue so we are using
			// a regex match and replace instead
			// Remove this workaround once https://github.com/envoyproxy/envoy/issues/26055 is fixed
			if routeCtx.Match.Path != nil && routeCtx.Match.Path.Value != nil && *config.Path.ReplacePrefixMatch == "/" {
				outputRoute.GetOptions().RegexRewrite = &matcherv3.RegexMatchAndSubstitute{
					Pattern: &matcherv3.RegexMatcher{
						Regex: "^" + *routeCtx.Match.Path.Value + `\/*`,
					},
					Substitution: "/",
				}
			} else {
				outputRoute.GetOptions().PrefixRewrite = &wrapperspb.StringValue{
					Value: *config.Path.ReplacePrefixMatch,
				}
			}
		}
	}

	return nil
}
