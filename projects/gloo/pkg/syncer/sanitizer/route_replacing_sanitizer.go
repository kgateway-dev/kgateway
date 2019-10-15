package sanitizer

import (
	"context"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/stats"
	"go.opencensus.io/tag"
	"sort"

	"go.uber.org/zap"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/proto"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errors"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var (
	routeConfigKey, _ = tag.NewKey("route_config_name")

	mRoutesReplaced = utils.MakeLastValueCounter("gloo.solo.io/sanitizer/routes_replaced", "The number routes replaced in the sanitized xds snapshot", stats.ProxyNameKey, routeConfigKey)
)

type RouteReplacingSanitizer struct {
	enabled                bool
	replacementRouteAction *envoyroute.DirectResponseAction
}

func NewRouteReplacingSanitizer(cfg *v1.GlooOptions_InvalidConfigPolicy) *RouteReplacingSanitizer {
	return &RouteReplacingSanitizer{
		enabled: cfg.GetReplaceInvalidRoutes(),
		replacementRouteAction: &envoyroute.DirectResponseAction{
			Status: cfg.GetInvalidRouteResponseCode(),
			Body: &core.DataSource{
				Specifier: &core.DataSource_InlineString{
					InlineString: cfg.GetInvalidRouteResponseBody(),
				},
			},
		},
	}
}

func (s *RouteReplacingSanitizer) SanitizeSnapshot(ctx context.Context, glooSnapshot *v1.ApiSnapshot, xdsSnapshot envoycache.Snapshot, reports reporter.ResourceReports) (envoycache.Snapshot, error) {
	if !s.enabled {
		return xdsSnapshot, nil
	}

	ctx = contextutils.WithLogger(ctx, "invalid-route-replacer")

	contextutils.LoggerFrom(ctx).Debug("replacing routes which point to missing or errored upstreams with a direct response action")

	routeConfigs, err := getRoutes(xdsSnapshot)
	if err != nil {
		return nil, err
	}

	// mark all valid destination clusters
	validClusters := getValidClusters(glooSnapshot, reports)

	replacedRouteConfigs := s.replaceMissingClusterRoutes(ctx, validClusters, routeConfigs)

	xdsSnapshot = xds.NewSnapshotFromResources(
		xdsSnapshot.GetResources(xds.EndpointType),
		xdsSnapshot.GetResources(xds.ClusterType),
		translator.MakeRdsResources(replacedRouteConfigs),
		xdsSnapshot.GetResources(xds.ListenerType),
	)

	// If the snapshot is not consistent, error
	if err := xdsSnapshot.Consistent(); err != nil {
		return xdsSnapshot, err
	}

	return xdsSnapshot, nil
}

func getRoutes(snap envoycache.Snapshot) ([]*envoyapi.RouteConfiguration, error) {
	routeConfigProtos := snap.GetResources(xds.RouteType)
	var routeConfigs []*envoyapi.RouteConfiguration

	for _, routeConfigProto := range routeConfigProtos.Items {
		routeConfig, ok := routeConfigProto.ResourceProto().(*envoyapi.RouteConfiguration)
		if !ok {
			return nil, errors.Errorf("invalid type, expected *envoyapi.RouteConfiguration, found %T", routeConfigProto)
		}
		routeConfigs = append(routeConfigs, routeConfig)
	}

	sort.SliceStable(routeConfigs, func(i, j int) bool {
		return routeConfigs[i].GetName() < routeConfigs[j].GetName()
	})

	return routeConfigs, nil
}

func getValidClusters(snap *v1.ApiSnapshot, reports reporter.ResourceReports) map[string]struct{} {
	// mark all valid destination clusters
	validClusters := make(map[string]struct{})
	for _, up := range snap.Upstreams.AsInputResources() {
		if reports[up].Errors != nil {
			continue
		}
		clusterName := translator.UpstreamToClusterName(up.GetMetadata().Ref())
		validClusters[clusterName] = struct{}{}
	}
	return validClusters
}

func (s *RouteReplacingSanitizer) replaceMissingClusterRoutes(ctx context.Context, validClusters map[string]struct{}, routeConfigs []*envoyapi.RouteConfiguration) []*envoyapi.RouteConfiguration {
	var sanitizedRouteConfigs []*envoyapi.RouteConfiguration

	isInvalid := func(cluster string) bool {
		_, ok := validClusters[cluster]
		return !ok
	}

	debugW := contextutils.LoggerFrom(ctx).Debugw

	// replace any routes which do not point to a valid destination cluster
	for _, cfg := range routeConfigs {
		var replaced int64
		sanitizedRouteConfig := proto.Clone(cfg).(*envoyapi.RouteConfiguration)

		for i, vh := range sanitizedRouteConfig.GetVirtualHosts() {
			for j, route := range vh.GetRoutes() {
				routeAction := route.GetRoute()
				if routeAction == nil {
					continue
				}
				switch action := routeAction.GetClusterSpecifier().(type) {
				case *envoyroute.RouteAction_Cluster:
					if isInvalid(action.Cluster) {
						debugW("replacing route in virtual host with invalid cluster",
							zap.Any("cluster", action.Cluster), zap.Any("route", j), zap.Any("virtualhost", i))
						s.replaceRouteAction(&route)
						replaced++
					}
				case *envoyroute.RouteAction_WeightedClusters:
					for _, weightedCluster := range action.WeightedClusters.GetClusters() {
						if isInvalid(weightedCluster.GetName()) {
							debugW("replacing route in virtual host with invalid weighted cluster",
								zap.Any("cluster", weightedCluster.GetName()), zap.Any("route", j), zap.Any("virtualhost", i))

							s.replaceRouteAction(&route)
							replaced++
							break // only need to have one invalid cluster to get replaced
						}
					}
				default:
					continue
				}
				vh.Routes[j] = route
			}
			sanitizedRouteConfig.VirtualHosts[i] = vh
		}

		utils.Measure(ctx, mRoutesReplaced, replaced, tag.Insert(routeConfigKey, sanitizedRouteConfig.GetName()))
		sanitizedRouteConfigs = append(sanitizedRouteConfigs, sanitizedRouteConfig)
	}

	return sanitizedRouteConfigs
}

func (s *RouteReplacingSanitizer) replaceRouteAction(route *envoyroute.Route) {
	route.Action = &envoyroute.Route_DirectResponse{
		DirectResponse: s.replacementRouteAction,
	}
}
