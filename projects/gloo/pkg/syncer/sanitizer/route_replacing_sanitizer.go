package sanitizer

import (
	"context"
	"sort"

	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/golang/protobuf/ptypes"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/util"

	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/stats"
	"go.opencensus.io/tag"

	"go.uber.org/zap"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
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

const (
	fallbackListenerName   = "fallback_listener_for_invalid_routes"
	fallbackListenerSocket = "@" + fallbackListenerName
	fallbackClusterName    = "fallback_cluster_for_invalid_routes"
)

var (
	routeConfigKey, _ = tag.NewKey("route_config_name")

	mRoutesReplaced = utils.MakeLastValueCounter("gloo.solo.io/sanitizer/routes_replaced", "The number routes replaced in the sanitized xds snapshot", stats.ProxyNameKey, routeConfigKey)
)

type RouteReplacingSanitizer struct {
	enabled          bool
	fallbackListener *envoyapi.Listener
	fallbackCluster  *envoyapi.Cluster
}

func NewRouteReplacingSanitizer(cfg *v1.GlooOptions_InvalidConfigPolicy) (*RouteReplacingSanitizer, error) {

	responseCode := cfg.GetInvalidRouteResponseCode()
	responseBody := cfg.GetInvalidRouteResponseBody()

	listener, cluster, err := makeFallbackListenerAndCluster(responseCode, responseBody)
	if err != nil {
		return nil, err
	}

	return &RouteReplacingSanitizer{
		enabled:          cfg.GetReplaceInvalidRoutes(),
		fallbackListener: listener,
		fallbackCluster:  cluster,
	}, nil
}

func makeFallbackListenerAndCluster(responseCode uint32, responseBody string) (*envoyapi.Listener, *envoyapi.Cluster, error) {
	hcmConfig := &v2.HttpConnectionManager{
		CodecType:  v2.HttpConnectionManager_AUTO,
		StatPrefix: fallbackListenerName,
		RouteSpecifier: &v2.HttpConnectionManager_RouteConfig{
			RouteConfig: &envoyapi.RouteConfiguration{
				Name: "fallback_routes",
				VirtualHosts: []*envoyroute.VirtualHost{{
					Name:    "fallback_virtualhost",
					Domains: []string{"*"},
					Routes: []*envoyroute.Route{{
						Match: &envoyroute.RouteMatch{
							PathSpecifier: &envoyroute.RouteMatch_Prefix{
								Prefix: "/",
							},
						},
						Action: &envoyroute.Route_DirectResponse{
							DirectResponse: &envoyroute.DirectResponseAction{
								Status: responseCode,
								Body: &core.DataSource{
									Specifier: &core.DataSource_InlineString{
										InlineString: responseBody,
									},
								},
							},
						},
					}},
				}},
			},
		},
		HttpFilters: []*v2.HttpFilter{{
			Name: util.Router,
		}},
	}

	typedHcmConfig, err := ptypes.MarshalAny(hcmConfig)
	if err != nil {
		return nil, nil, err
	}

	fallbackListener := &envoyapi.Listener{
		Name: fallbackListenerName,
		Address: &core.Address{
			Address: &core.Address_Pipe{
				Pipe: &core.Pipe{
					Path: fallbackListenerSocket,
				},
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: util.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: typedHcmConfig,
				},
			}},
		}},
	}

	fallbackCluster := &envoyapi.Cluster{
		Name:           fallbackClusterName,
		ConnectTimeout: gogoutils.DurationStdToProto(&translator.ClusterConnectionTimeout),
		LoadAssignment: &envoyapi.ClusterLoadAssignment{
			ClusterName: fallbackClusterName,
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: []*endpoint.LbEndpoint{{
					HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{
							Address: &core.Address{
								Address: &core.Address_Pipe{
									Pipe: &core.Pipe{
										Path: fallbackListenerSocket,
									},
								},
							},
						},
					},
				}},
			}},
		},
	}

	return fallbackListener, fallbackCluster, nil
}

func (s *RouteReplacingSanitizer) SanitizeSnapshot(ctx context.Context, glooSnapshot *v1.ApiSnapshot, xdsSnapshot envoycache.Snapshot, reports reporter.ResourceReports) (envoycache.Snapshot, error) {
	if !s.enabled {
		// if if the route sanitizer is not enabled, enforce strict validation of routes (warnings are treated as errors)
		// this is necessary because the translator only uses Validate() which ignores warnings
		return xdsSnapshot, reports.ValidateStrict()
	}

	ctx = contextutils.WithLogger(ctx, "invalid-route-replacer")

	contextutils.LoggerFrom(ctx).Debug("replacing routes which point to missing or errored upstreams with a direct response action")

	routeConfigs, err := getRoutes(xdsSnapshot)
	if err != nil {
		return nil, err
	}

	// mark all valid destination clusters
	validClusters := getClusters(glooSnapshot)

	replacedRouteConfigs, needsListener := s.replaceMissingClusterRoutes(ctx, validClusters, routeConfigs)

	clusters := xdsSnapshot.GetResources(xds.ClusterType)
	listeners := xdsSnapshot.GetResources(xds.ListenerType)

	if needsListener {
		s.insertFallbackListener(&listeners)
		s.insertFallbackCluster(&clusters)
	}

	xdsSnapshot = xds.NewSnapshotFromResources(
		xdsSnapshot.GetResources(xds.EndpointType),
		clusters,
		translator.MakeRdsResources(replacedRouteConfigs),
		listeners,
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

func getClusters(snap *v1.ApiSnapshot) map[string]struct{} {
	// mark all valid destination clusters
	validClusters := make(map[string]struct{})
	for _, up := range snap.Upstreams.AsInputResources() {
		clusterName := translator.UpstreamToClusterName(up.GetMetadata().Ref())
		validClusters[clusterName] = struct{}{}
	}
	return validClusters
}

func (s *RouteReplacingSanitizer) replaceMissingClusterRoutes(ctx context.Context, validClusters map[string]struct{}, routeConfigs []*envoyapi.RouteConfiguration) ([]*envoyapi.RouteConfiguration, bool) {
	var sanitizedRouteConfigs []*envoyapi.RouteConfiguration

	isInvalid := func(cluster string) bool {
		_, ok := validClusters[cluster]
		return !ok
	}

	debugW := contextutils.LoggerFrom(ctx).Debugw

	var anyRoutesReplaced bool

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
						action.Cluster = s.fallbackCluster.Name
						replaced++
						anyRoutesReplaced = true
					}
				case *envoyroute.RouteAction_WeightedClusters:
					for _, weightedCluster := range action.WeightedClusters.GetClusters() {
						if isInvalid(weightedCluster.GetName()) {
							debugW("replacing route in virtual host with invalid weighted cluster",
								zap.Any("cluster", weightedCluster.GetName()), zap.Any("route", j), zap.Any("virtualhost", i))

							weightedCluster.Name = s.fallbackCluster.Name
							replaced++
							anyRoutesReplaced = true
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

	return sanitizedRouteConfigs, anyRoutesReplaced
}

func (s *RouteReplacingSanitizer) insertFallbackListener(listeners *envoycache.Resources) {
	if listeners.Items == nil {
		listeners.Items = map[string]envoycache.Resource{}
	}

	listener := xds.NewEnvoyResource(s.fallbackListener)

	listeners.Items[listener.Self().Name] = listener
	listeners.Version += "-with-fallback-listener"
}

func (s *RouteReplacingSanitizer) insertFallbackCluster(clusters *envoycache.Resources) {
	if clusters.Items == nil {
		clusters.Items = map[string]envoycache.Resource{}
	}

	cluster := xds.NewEnvoyResource(s.fallbackCluster)

	clusters.Items[cluster.Self().Name] = cluster
	clusters.Version += "-with-fallback-cluster"
}
