package translator

import (
	"fmt"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"

	"github.com/gogo/protobuf/proto"
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"

	usconversion "github.com/solo-io/gloo/projects/gloo/pkg/upstreams"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var (
	NoDestinationSpecifiedError = errors.New("must specify at least one weighted destination for multi destination routes")

	SubsetsMisconfiguredErr = errors.New("route has a subset config, but the upstream does not.")
)

func (t *translator) computeRouteConfig(params plugins.Params, proxy *v1.Proxy, listener *v1.Listener, routeCfgName string, listenerReport *validationapi.ListenerReport) *envoyapi.RouteConfiguration {
	if listener.GetHttpListener() == nil {
		return nil
	}

	httpListenerReport := listenerReport.GetHttpListenerReport()
	if httpListenerReport == nil {
		contextutils.LoggerFrom(params.Ctx).DPanic("internal error: listener report was not http type")
	}

	params.Ctx = contextutils.WithLogger(params.Ctx, "compute_route_config."+routeCfgName)

	virtualHosts := t.computeVirtualHosts(params, proxy, listener, httpListenerReport)

	// validate ssl config if the listener specifies any
	if err := validateListenerSslConfig(params, listener); err != nil {
		validation.AppendListenerError(listenerReport,
			validationapi.ListenerReport_Error_SSLConfigError,
			err.Error(),
		)
	}

	return &envoyapi.RouteConfiguration{
		Name:         routeCfgName,
		VirtualHosts: virtualHosts,
	}
}

func (t *translator) computeVirtualHosts(params plugins.Params, proxy *v1.Proxy, listener *v1.Listener, httpListenerReport *validationapi.HttpListenerReport) []envoyroute.VirtualHost {
	httpListener, ok := listener.ListenerType.(*v1.Listener_HttpListener)
	if !ok {
		return nil
	}
	virtualHosts := httpListener.HttpListener.VirtualHosts
	validateVirtualHostDomains(virtualHosts, httpListenerReport)
	requireTls := len(listener.SslConfigurations) > 0
	var envoyVirtualHosts []envoyroute.VirtualHost
	for i, virtualHost := range virtualHosts {
		vhostParams := plugins.VirtualHostParams{
			Params:   params,
			Listener: listener,
			Proxy:    proxy,
		}
		vhostReport := httpListenerReport.VirtualHostReports[i]
		envoyVirtualHosts = append(envoyVirtualHosts, t.computeVirtualHost(vhostParams, virtualHost, requireTls, vhostReport))
	}
	return envoyVirtualHosts
}

func (t *translator) computeVirtualHost(params plugins.VirtualHostParams, virtualHost *v1.VirtualHost, requireTls bool, vhostReport *validationapi.VirtualHostReport) envoyroute.VirtualHost {

	// Make copy to avoid modifying the snapshot
	virtualHost = proto.Clone(virtualHost).(*v1.VirtualHost)
	virtualHost.Name = utils.SanitizeForEnvoy(params.Ctx, virtualHost.Name, "virtual host")

	var envoyRoutes []envoyroute.Route
	for i, route := range virtualHost.Routes {
		routeParams := plugins.RouteParams{
			VirtualHostParams: params,
			VirtualHost:       virtualHost,
		}
		routeReport := vhostReport.RouteReports[i]
		envoyRoute := t.envoyRoute(routeParams, routeReport, route)
		envoyRoutes = append(envoyRoutes, envoyRoute)
	}
	domains := virtualHost.Domains
	if len(domains) == 0 || (len(domains) == 1 && domains[0] == "") {
		domains = []string{"*"}
	}
	var envoyRequireTls envoyroute.VirtualHost_TlsRequirementType
	if requireTls {
		// TODO (ilackarms): support external-only TLS
		envoyRequireTls = envoyroute.VirtualHost_ALL
	}

	out := envoyroute.VirtualHost{
		Name:       virtualHost.Name,
		Domains:    domains,
		Routes:     envoyRoutes,
		RequireTls: envoyRequireTls,
	}

	// run the plugins
	for _, plug := range t.plugins {
		virtualHostPlugin, ok := plug.(plugins.VirtualHostPlugin)
		if !ok {
			continue
		}
		if err := virtualHostPlugin.ProcessVirtualHost(params, virtualHost, &out); err != nil {
			validation.AppendVirtualHostError(
				vhostReport,
				validationapi.VirtualHostReport_Error_ProcessingError,
				fmt.Sprintf("invalid virtual host [%s]: %v", virtualHost.Name, err.Error()),
			)
		}
	}
	return out
}

func (t *translator) envoyRoute(params plugins.RouteParams, routeReport *validationapi.RouteReport, in *v1.Route) envoyroute.Route {
	out := &envoyroute.Route{}

	setMatch(in, routeReport, out)

	t.setAction(params, routeReport, in, out)

	return *out
}

func setMatch(in *v1.Route, routeReport *validationapi.RouteReport, out *envoyroute.Route) {

	if in.Matcher == nil {
		validation.AppendRouteError(routeReport,
			validationapi.RouteReport_Error_InvalidMatcherError,
			"no matcher provided",
		)
	}
	if in.Matcher.PathSpecifier == nil {
		validation.AppendRouteError(routeReport,
			validationapi.RouteReport_Error_InvalidMatcherError,
			"no path specifier provided",
		)
	}
	match := envoyroute.RouteMatch{
		Headers:         envoyHeaderMatcher(in.Matcher.Headers),
		QueryParameters: envoyQueryMatcher(in.Matcher.QueryParameters),
	}
	if len(in.Matcher.Methods) > 0 {
		match.Headers = append(match.Headers, &envoyroute.HeaderMatcher{
			Name: ":method",
			HeaderMatchSpecifier: &envoyroute.HeaderMatcher_RegexMatch{
				RegexMatch: strings.Join(in.Matcher.Methods, "|"),
			},
		})
	}
	// need to do this because Go's proto implementation makes oneofs private
	// which genius thought of that?
	setEnvoyPathMatcher(in.Matcher, &match)

	out.Match = match
}

func (t *translator) setAction(params plugins.RouteParams, routeReport *validationapi.RouteReport, in *v1.Route, out *envoyroute.Route) {
	switch action := in.Action.(type) {
	case *v1.Route_RouteAction:
		if err := ValidateRouteDestinations(params.Snapshot, action.RouteAction); err != nil {
			validation.AppendRouteWarning(routeReport,
				validationapi.RouteReport_Warning_InvalidDestinationWarning,
				err.Error(),
			)
		}

		out.Action = &envoyroute.Route_Route{
			Route: &envoyroute.RouteAction{},
		}
		if err := t.setRouteAction(params, action.RouteAction, out.Action.(*envoyroute.Route_Route).Route, routeReport); err != nil {
			if isWarningErr(err) {
				validation.AppendRouteWarning(routeReport,
					validationapi.RouteReport_Warning_InvalidDestinationWarning,
					err.Error(),
				)
			} else {
				validation.AppendRouteError(routeReport,
					validationapi.RouteReport_Error_ProcessingError,
					err.Error(),
				)
			}
		}

		// run the plugins for RoutePlugin
		for _, plug := range t.plugins {
			routePlugin, ok := plug.(plugins.RoutePlugin)
			if !ok {
				continue
			}
			if err := routePlugin.ProcessRoute(params, in, out); err != nil {
				// plugins can return errors on missing upstream/upstream group
				// we only want to report errors that are plugin-specific
				// missing upstream(group) should produce a warning above
				if isWarningErr(err) {
					continue
				}
				validation.AppendRouteError(routeReport,
					validationapi.RouteReport_Error_ProcessingError,
					fmt.Sprintf("%T: %v", routePlugin, err.Error()),
				)
			}
		}

		// run the plugins for RouteActionPlugin
		for _, plug := range t.plugins {
			routePlugin, ok := plug.(plugins.RouteActionPlugin)
			if !ok || in.GetRouteAction() == nil || out.GetRoute() == nil {
				continue
			}
			raParams := plugins.RouteActionParams{
				RouteParams: params,
				Route:       in,
			}
			if err := routePlugin.ProcessRouteAction(raParams, in.GetRouteAction(), out.GetRoute()); err != nil {
				// same as above
				if isWarningErr(err) {
					continue
				}
				validation.AppendRouteError(routeReport,
					validationapi.RouteReport_Error_ProcessingError,
					err.Error(),
				)
			}
		}

	case *v1.Route_DirectResponseAction:
		out.Action = &envoyroute.Route_DirectResponse{
			DirectResponse: &envoyroute.DirectResponseAction{
				Status: action.DirectResponseAction.Status,
				Body:   DataSourceFromString(action.DirectResponseAction.Body),
			},
		}

	case *v1.Route_RedirectAction:
		out.Action = &envoyroute.Route_Redirect{
			Redirect: &envoyroute.RedirectAction{
				HostRedirect:           action.RedirectAction.HostRedirect,
				ResponseCode:           envoyroute.RedirectAction_RedirectResponseCode(action.RedirectAction.ResponseCode),
				SchemeRewriteSpecifier: &envoyroute.RedirectAction_HttpsRedirect{HttpsRedirect: action.RedirectAction.HttpsRedirect},
				StripQuery:             action.RedirectAction.StripQuery,
			},
		}

		switch pathRewrite := action.RedirectAction.PathRewriteSpecifier.(type) {
		case *v1.RedirectAction_PathRedirect:
			out.Action.(*envoyroute.Route_Redirect).Redirect.PathRewriteSpecifier = &envoyroute.RedirectAction_PathRedirect{
				PathRedirect: pathRewrite.PathRedirect,
			}
		case *v1.RedirectAction_PrefixRewrite:
			out.Action.(*envoyroute.Route_Redirect).Redirect.PathRewriteSpecifier = &envoyroute.RedirectAction_PrefixRewrite{
				PrefixRewrite: pathRewrite.PrefixRewrite,
			}
		}
	}
}

func (t *translator) setRouteAction(params plugins.RouteParams, in *v1.RouteAction, out *envoyroute.RouteAction, routeReport *validationapi.RouteReport) error {
	switch dest := in.Destination.(type) {
	case *v1.RouteAction_Single:
		usRef, err := usconversion.DestinationToUpstreamRef(dest.Single)
		if err != nil {
			return err
		}
		out.ClusterSpecifier = &envoyroute.RouteAction_Cluster{
			Cluster: UpstreamToClusterName(*usRef),
		}

		out.MetadataMatch = getSubsetMatch(dest.Single)

		return checkThatSubsetMatchesUpstream(params.Params, dest.Single)
	case *v1.RouteAction_Multi:
		return t.setWeightedClusters(params, dest.Multi, out, routeReport)
	case *v1.RouteAction_UpstreamGroup:
		upstreamGroupRef := dest.UpstreamGroup
		upstreamGroup, err := params.Snapshot.UpstreamGroups.Find(upstreamGroupRef.Namespace, upstreamGroupRef.Name)
		if err != nil {
			return pluginutils.NewUpstreamGroupNotFoundErr(*upstreamGroupRef)
		}
		md := &v1.MultiDestination{
			Destinations: upstreamGroup.Destinations,
		}
		return t.setWeightedClusters(params, md, out, routeReport)
	}
	return errors.Errorf("unknown upstream destination type")
}

func (t *translator) setWeightedClusters(params plugins.RouteParams, multiDest *v1.MultiDestination, out *envoyroute.RouteAction, routeReport *validationapi.RouteReport) error {
	if len(multiDest.Destinations) == 0 {
		return NoDestinationSpecifiedError
	}

	clusterSpecifier := &envoyroute.RouteAction_WeightedClusters{
		WeightedClusters: &envoyroute.WeightedCluster{},
	}

	var totalWeight uint32
	for _, weightedDest := range multiDest.Destinations {

		usRef, err := usconversion.DestinationToUpstreamRef(weightedDest.Destination)
		if err != nil {
			return err
		}

		totalWeight += weightedDest.Weight

		weightedCluster := &envoyroute.WeightedCluster_ClusterWeight{
			Name:          UpstreamToClusterName(*usRef),
			Weight:        &types.UInt32Value{Value: weightedDest.Weight},
			MetadataMatch: getSubsetMatch(weightedDest.Destination),
		}

		// run the plugins for Weighted Destinations
		for _, plug := range t.plugins {
			weightedDestinationPlugin, ok := plug.(plugins.WeightedDestinationPlugin)
			if !ok {
				continue
			}
			if err := weightedDestinationPlugin.ProcessWeightedDestination(params, weightedDest, weightedCluster); err != nil {
				validation.AppendRouteError(routeReport,
					validationapi.RouteReport_Error_ProcessingError,
					err.Error(),
				)
			}
		}

		clusterSpecifier.WeightedClusters.Clusters = append(clusterSpecifier.WeightedClusters.Clusters, weightedCluster)

		if err = checkThatSubsetMatchesUpstream(params.Params, weightedDest.Destination); err != nil {
			return err
		}
	}

	clusterSpecifier.WeightedClusters.TotalWeight = &types.UInt32Value{Value: totalWeight}

	out.ClusterSpecifier = clusterSpecifier
	return nil
}

// TODO(marco): when we update the routing API we should move this to a RouteActionPlugin
func getSubsetMatch(destination *v1.Destination) *envoycore.Metadata {
	var routeMetadata *envoycore.Metadata

	// TODO(yuval-k): should we add validation that the route subset indeed exists in the upstream?
	// First convert the subset information on the base destination, if present
	if destination.Subset != nil {
		routeMetadata = getLbMetadata(nil, destination.Subset.Values, "")
	}
	return routeMetadata
}

func checkThatSubsetMatchesUpstream(params plugins.Params, dest *v1.Destination) error {

	// make sure we have a subset config on the route
	if dest.Subset == nil {
		return nil
	}
	if len(dest.Subset.Values) == 0 {
		return nil
	}
	routeSubset := dest.Subset.Values

	ref, err := usconversion.DestinationToUpstreamRef(dest)
	if err != nil {
		return err
	}

	upstream, err := params.Snapshot.Upstreams.Find(ref.Namespace, ref.Name)
	if err != nil {
		return pluginutils.NewUpstreamNotFoundErr(*ref)
	}

	subsetConfig := getSubsets(upstream)

	// if a route has a subset config, and an upstream doesnt - its an error
	if subsetConfig == nil {
		return SubsetsMisconfiguredErr
	}

	// make sure that the subset on the route will match a subset on the upstream.
	found := false
Outerloop:
	for _, subset := range subsetConfig.Selectors {
		keys := subset.Keys
		if len(keys) != len(routeSubset) {
			continue
		}
		for _, k := range keys {
			if _, ok := routeSubset[k]; !ok {
				continue Outerloop
			}
		}
		found = true
		break
	}

	if !found {
		return errors.Errorf("route has a subset config, but none of the subsets in the upstream match it.")

	}
	return nil
}

func getSubsets(upstream *v1.Upstream) *v1plugins.SubsetSpec {

	specGetter, ok := upstream.UpstreamSpec.UpstreamType.(v1.SubsetSpecGetter)
	if !ok {
		return nil
	}
	glooSubsetConfig := specGetter.GetSubsetSpec()

	return glooSubsetConfig

}

func setEnvoyPathMatcher(in *v1.Matcher, out *envoyroute.RouteMatch) {
	switch path := in.PathSpecifier.(type) {
	case *v1.Matcher_Exact:
		out.PathSpecifier = &envoyroute.RouteMatch_Path{
			Path: path.Exact,
		}
	case *v1.Matcher_Regex:
		out.PathSpecifier = &envoyroute.RouteMatch_Regex{
			Regex: path.Regex,
		}
	case *v1.Matcher_Prefix:
		out.PathSpecifier = &envoyroute.RouteMatch_Prefix{
			Prefix: path.Prefix,
		}
	}
}

func envoyHeaderMatcher(in []*v1.HeaderMatcher) []*envoyroute.HeaderMatcher {
	var out []*envoyroute.HeaderMatcher
	for _, matcher := range in {

		envoyMatch := &envoyroute.HeaderMatcher{
			Name: matcher.Name,
		}
		if matcher.Value == "" {
			envoyMatch.HeaderMatchSpecifier = &envoyroute.HeaderMatcher_PresentMatch{
				PresentMatch: true,
			}
		} else {

			envoyMatch.HeaderMatchSpecifier = &envoyroute.HeaderMatcher_ExactMatch{
				ExactMatch: matcher.Value,
			}
			if matcher.Regex {
				envoyMatch.HeaderMatchSpecifier = &envoyroute.HeaderMatcher_RegexMatch{
					RegexMatch: matcher.Value,
				}
			} else {
				envoyMatch.HeaderMatchSpecifier = &envoyroute.HeaderMatcher_ExactMatch{
					ExactMatch: matcher.Value,
				}
			}
		}
		out = append(out, envoyMatch)
	}
	return out
}

func envoyQueryMatcher(in []*v1.QueryParameterMatcher) []*envoyroute.QueryParameterMatcher {
	var out []*envoyroute.QueryParameterMatcher
	for _, matcher := range in {
		envoyMatch := &envoyroute.QueryParameterMatcher{
			Name:  matcher.Name,
			Value: matcher.Value,
			Regex: &types.BoolValue{
				Value: matcher.Regex,
			},
		}
		out = append(out, envoyMatch)
	}
	return out
}

// returns an error if any of the virtualhost domains overlap
func validateVirtualHostDomains(virtualHosts []*v1.VirtualHost, httpListenerReport *validationapi.HttpListenerReport) {
	// this shouldbe a 1-1 mapping
	// if len(domainsToVirtualHosts[domain]) > 1, it's an error
	domainsToVirtualHosts := make(map[string][]int)
	for i, vHost := range virtualHosts {
		if len(vHost.Domains) == 0 {
			// default virtualhost
			domainsToVirtualHosts["*"] = append(domainsToVirtualHosts["*"], i)
		}
		for _, domain := range vHost.Domains {
			// default virtualhost can be specified with empty string
			if domain == "" {
				domain = "*"
			}
			domainsToVirtualHosts[domain] = append(domainsToVirtualHosts[domain], i)
		}
	}
	// see if we found any conflicts, if so, write reports
	for domain, vHosts := range domainsToVirtualHosts {
		if len(vHosts) > 1 {
			var vHostNames []string
			// collect names of all vhosts with the domain
			for _, vHost := range vHosts {
				vHostNames = append(vHostNames, virtualHosts[vHost].Name)
			}

			// append errors for this vhost
			for _, vHost := range vHosts {
				vhostReport := httpListenerReport.VirtualHostReports[vHost]
				validation.AppendVirtualHostError(
					vhostReport,
					validationapi.VirtualHostReport_Error_DomainsNotUniqueError,
					fmt.Sprintf("domain %v is "+
						"shared by the following virtual hosts: %v", domain, vHostNames),
				)
			}
		}
	}
}

func ValidateRouteDestinations(snap *v1.ApiSnapshot, action *v1.RouteAction) error {
	upstreams := snap.Upstreams
	// make sure the destination itself has the right structure
	switch dest := action.Destination.(type) {
	case *v1.RouteAction_Single:
		return validateSingleDestination(upstreams, dest.Single)
	case *v1.RouteAction_Multi:
		return validateMultiDestination(upstreams, dest.Multi.Destinations)
	case *v1.RouteAction_UpstreamGroup:
		return validateUpstreamGroup(snap, dest.UpstreamGroup)
	}
	return errors.Errorf("must specify either 'singleDestination', 'multipleDestinations' or 'upstreamGroup' for action")
}

func validateUpstreamGroup(snap *v1.ApiSnapshot, ref *core.ResourceRef) error {

	upstreamGroup, err := snap.UpstreamGroups.Find(ref.Namespace, ref.Name)
	if err != nil {
		return pluginutils.NewUpstreamGroupNotFoundErr(*ref)
	}
	upstreams := snap.Upstreams

	err = validateMultiDestination(upstreams, upstreamGroup.Destinations)
	if err != nil {
		return err
	}
	return nil
}

func validateMultiDestination(upstreams []*v1.Upstream, destinations []*v1.WeightedDestination) error {
	for _, dest := range destinations {
		if err := validateSingleDestination(upstreams, dest.Destination); err != nil {
			return errors.Wrap(err, "invalid destination in weighted destination list")
		}
	}
	return nil
}

func validateSingleDestination(upstreams v1.UpstreamList, destination *v1.Destination) error {
	upstreamRef, err := usconversion.DestinationToUpstreamRef(destination)
	if err != nil {
		return err
	}
	_, err = upstreams.Find(upstreamRef.Strings())
	if err != nil {
		return pluginutils.NewUpstreamNotFoundErr(*upstreamRef)
	}
	return nil
}

func validateListenerSslConfig(params plugins.Params, listener *v1.Listener) error {
	sslCfgTranslator := utils.NewSslConfigTranslator()
	for _, ssl := range listener.SslConfigurations {
		if _, err := sslCfgTranslator.ResolveDownstreamSslConfig(params.Snapshot.Secrets, ssl); err != nil {
			return err
		}
	}
	return nil
}

func DataSourceFromString(str string) *envoycore.DataSource {
	return &envoycore.DataSource{
		Specifier: &envoycore.DataSource_InlineString{
			InlineString: str,
		},
	}
}

func isWarningErr(err error) bool {
	switch {
	case err == SubsetsMisconfiguredErr:
		fallthrough
	case pluginutils.IsDestinationNotFoundErr(err):
		return true
	default:
		return false
	}
}
