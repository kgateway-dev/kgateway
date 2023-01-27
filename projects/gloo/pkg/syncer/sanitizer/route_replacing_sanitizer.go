package sanitizer

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/constants"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"go.opencensus.io/tag"
	"go.uber.org/zap"
)

const (
	fallbackListenerName   = "fallback_listener_for_invalid_routes"
	fallbackListenerSocket = "@" + fallbackListenerName
	fallbackClusterName    = "fallback_cluster_for_invalid_routes"
)

var (
	routeConfigKey, _ = tag.NewKey("route_config_name")
	mRoutesReplaced   = utils.MakeLastValueCounter("gloo.solo.io/sanitizer/routes_replaced", "The number routes replaced in the sanitized xds snapshot", stats.ProxyNameKey, routeConfigKey)

	// Compile-time assertion
	_ XdsSanitizer = new(RouteReplacingSanitizer)
)

type RouteReplacingSanitizer struct {
	// note to devs: this can be called in parallel by the validation webhook and main translation loops at the same time
	// any stateful fields should be protected by a mutex
	enabled          bool
	fallbackListener *envoy_config_listener_v3.Listener
	fallbackCluster  *envoy_config_cluster_v3.Cluster
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

func makeFallbackListenerAndCluster(
	responseCode uint32,
	responseBody string,
) (*envoy_config_listener_v3.Listener, *envoy_config_cluster_v3.Cluster, error) {
	hcmConfig := &envoyhcm.HttpConnectionManager{
		CodecType:  envoyhcm.HttpConnectionManager_AUTO,
		StatPrefix: fallbackListenerName,
		RouteSpecifier: &envoyhcm.HttpConnectionManager_RouteConfig{
			RouteConfig: &envoy_config_route_v3.RouteConfiguration{
				Name: "fallback_routes",
				VirtualHosts: []*envoy_config_route_v3.VirtualHost{{
					Name:    "fallback_virtualhost",
					Domains: []string{"*"},
					Routes: []*envoy_config_route_v3.Route{{
						Match: &envoy_config_route_v3.RouteMatch{
							PathSpecifier: &envoy_config_route_v3.RouteMatch_Prefix{
								Prefix: "/",
							},
						},
						Action: &envoy_config_route_v3.Route_DirectResponse{
							DirectResponse: &envoy_config_route_v3.DirectResponseAction{
								Status: responseCode,
								Body: &envoy_config_core_v3.DataSource{
									Specifier: &envoy_config_core_v3.DataSource_InlineString{
										InlineString: responseBody,
									},
								},
							},
						},
					}},
				}},
			},
		},
		HttpFilters: []*envoyhcm.HttpFilter{
			{
				Name: wellknown.Router,
				ConfigType: &envoyhcm.HttpFilter_TypedConfig{
					TypedConfig: &any.Any{
						TypeUrl: "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router",
					},
				},
			}},
	}

	typedHcmConfig, err := glooutils.MessageToAny(hcmConfig)
	if err != nil {
		return nil, nil, err
	}

	fallbackListener := &envoy_config_listener_v3.Listener{
		Name: fallbackListenerName,
		Address: &envoy_config_core_v3.Address{
			Address: &envoy_config_core_v3.Address_Pipe{
				Pipe: &envoy_config_core_v3.Pipe{
					Path: fallbackListenerSocket,
				},
			},
		},
		FilterChains: []*envoy_config_listener_v3.FilterChain{{
			Filters: []*envoy_config_listener_v3.Filter{{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: typedHcmConfig,
				},
			}},
		}},
	}

	fallbackCluster := &envoy_config_cluster_v3.Cluster{
		Name:           fallbackClusterName,
		ConnectTimeout: ptypes.DurationProto(translator.ClusterConnectionTimeout),
		LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
			ClusterName: fallbackClusterName,
			Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{{
				LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{{
					HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
						Endpoint: &envoy_config_endpoint_v3.Endpoint{
							Address: &envoy_config_core_v3.Address{
								Address: &envoy_config_core_v3.Address_Pipe{
									Pipe: &envoy_config_core_v3.Pipe{
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

func (s *RouteReplacingSanitizer) SanitizeSnapshot(
	ctx context.Context,
	glooSnapshot *v1snap.ApiSnapshot,
	xdsSnapshot envoycache.Snapshot,
	reports reporter.ResourceReports,
) envoycache.Snapshot {
	if !s.enabled {
		return xdsSnapshot
	}

	ctx = contextutils.WithLogger(ctx, "invalid-route-replacer")

	contextutils.LoggerFrom(ctx).Debug("replacing routes which point to missing or errored upstreams with a direct response action")

	routeConfigs := getRoutes(ctx, xdsSnapshot)
	if len(routeConfigs) == 0 {
		contextutils.LoggerFrom(ctx).Debug("xds snapshot had no routes for route sanitizer to replace")
		return xdsSnapshot
	}

	// mark all valid destination clusters
	validClusters := getClusters(glooSnapshot, xdsSnapshot)

	proxyReports := reports.FilterByKind("*v1.Proxy")
	erroredRouteNames := s.removeErroredRoutesFromReport(proxyReports, reports)

	replacedRouteConfigs, needsListener := s.replaceRoutes(ctx, validClusters, routeConfigs, erroredRouteNames)

	clusters := xdsSnapshot.GetResources(types.ClusterTypeV3)
	listeners := xdsSnapshot.GetResources(types.ListenerTypeV3)

	if needsListener {
		s.insertFallbackListener(&listeners)
		s.insertFallbackCluster(&clusters)
	}

	newXdsSnapshot := xds.NewSnapshotFromResources(
		xdsSnapshot.GetResources(types.EndpointTypeV3),
		clusters,
		translator.MakeRdsResources(replacedRouteConfigs),
		listeners,
	)

	return newXdsSnapshot
}

func getRoutes(ctx context.Context, snap envoycache.Snapshot) []*envoy_config_route_v3.RouteConfiguration {
	routeConfigProtos := snap.GetResources(types.RouteTypeV3)
	var routeConfigs []*envoy_config_route_v3.RouteConfiguration

	for _, routeConfigProto := range routeConfigProtos.Items {
		routeConfig, ok := routeConfigProto.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
		if !ok {
			// should never happen
			contextutils.LoggerFrom(ctx).DPanicf("error: xds snapshot resources of type RouteTypeV3 were not "+
				"converted to *envoy_config_route_v3.RouteConfiguration, instead found %T", routeConfigProto.ResourceProto())
			return nil
		}
		routeConfigs = append(routeConfigs, routeConfig)
	}

	sort.SliceStable(routeConfigs, func(i, j int) bool {
		return routeConfigs[i].GetName() < routeConfigs[j].GetName()
	})

	return routeConfigs
}

func getClusters(glooSnapshot *v1snap.ApiSnapshot, xdsSnapshot envoycache.Snapshot) map[string]struct{} {
	// mark all valid destination clusters, i.e. those that are in both the gloo snapshot and xds snapshot
	validClusters := make(map[string]struct{})
	xdsClusters := xdsSnapshot.GetResources(types.ClusterTypeV3)

	for _, up := range glooSnapshot.Upstreams.AsInputResources() {
		clusterName := translator.UpstreamToClusterName(up.GetMetadata().Ref())
		if xdsClusters.Items[clusterName] != nil {
			validClusters[clusterName] = struct{}{}
		}
	}

	for clusterName := range xdsClusters.Items {
		// clusters created by gloo, such as dynamic forward proxy clusters, are always valid.
		// This loop _ensures_ they are, because the "exist in gloo snapshot's upstreams" isn't a valid assumption
		// for such clusters.  They can be identified with the prefix "solo_io_generated_"
		if strings.HasPrefix(clusterName, constants.SoloGeneratedClusterPrefix) {
			validClusters[clusterName] = struct{}{}
		}
		// in the future, if you are considering adding a new prefix here, please consider whether it is
		// possible to rename the cluster to use $SoloGeneratedClusterPrefix instead.
	}

	return validClusters
}

func (s *RouteReplacingSanitizer) replaceRoutes(
	ctx context.Context,
	validClusters map[string]struct{},
	routeConfigs []*envoy_config_route_v3.RouteConfiguration,
	erroredRoutes map[string]struct{},
) ([]*envoy_config_route_v3.RouteConfiguration, bool) {
	var sanitizedRouteConfigs []*envoy_config_route_v3.RouteConfiguration

	isInvalid := func(cluster string, name string) bool {
		_, valid := validClusters[cluster]
		_, errored := erroredRoutes[name]
		return !valid || errored
	}

	debugW := contextutils.LoggerFrom(ctx).Debugw

	var anyRoutesReplaced bool

	// replace any routes which do not point to a valid destination cluster
	for _, cfg := range routeConfigs {
		var replaced int64

		for i, vh := range cfg.GetVirtualHosts() {
			for j, route := range vh.GetRoutes() {
				routeAction := route.GetRoute()
				if routeAction == nil {
					continue
				}
				switch action := routeAction.GetClusterSpecifier().(type) {
				case *envoy_config_route_v3.RouteAction_Cluster:
					if isInvalid(action.Cluster, route.GetName()) {
						debugW("replacing route in virtual host with invalid cluster",
							zap.Any("cluster", action.Cluster), zap.Any("route", j), zap.Any("virtualhost", i))
						action.Cluster = s.fallbackCluster.GetName()
						replaced++
						anyRoutesReplaced = true
					}
				case *envoy_config_route_v3.RouteAction_WeightedClusters:
					for _, weightedCluster := range action.WeightedClusters.GetClusters() {
						if isInvalid(weightedCluster.GetName(), route.GetName()) {
							debugW("replacing route in virtual host with invalid weighted cluster",
								zap.Any("cluster", weightedCluster.GetName()), zap.Any("route", j), zap.Any("virtualhost", i))

							weightedCluster.Name = s.fallbackCluster.GetName()
							replaced++
							anyRoutesReplaced = true
						}
					}
				default:
					continue
				}
			}
		}

		utils.Measure(ctx, mRoutesReplaced, replaced, tag.Insert(routeConfigKey, cfg.GetName()))
		sanitizedRouteConfigs = append(sanitizedRouteConfigs, cfg)
	}

	return sanitizedRouteConfigs, anyRoutesReplaced
}

func (s *RouteReplacingSanitizer) removeErroredRoutesFromReport(
	proxyReports reporter.ResourceReports,
	allReports reporter.ResourceReports,
) map[string]struct{} {
	erroredRoutes := make(map[string]struct{})
	for proxy, report := range proxyReports {
		if report.Errors == nil {
			continue
		}

		// Break out multiple errors
		errors := report.Errors.(*multierror.Error).Errors
		modifiedReport := report
		remainingErrors := make([]error, 0)
		for _, proxyError := range errors {
			routeError := eris.New(validationutils.RouteErrorMsg)
			if eris.Is(proxyError, routeError) {
				proxyErrorStr := proxyError.Error()
				re := regexp.MustCompile(validationutils.RouteIdentifierTxt + ": (.*)")
				match := re.FindStringSubmatch(proxyErrorStr)
				if match != nil {
					erroredRoutes[match[1]] = struct{}{}
					modifiedReport.Warnings = append(modifiedReport.Warnings, proxyErrorStr)
				} else {
					remainingErrors = append(remainingErrors, proxyError)
				}
			} else {
				remainingErrors = append(remainingErrors, proxyError)
			}
		}
		if len(remainingErrors) > 0 {
			var multiErr *multierror.Error
			for _, remainingError := range remainingErrors {
				multiErr = multierror.Append(multiErr, remainingError)
			}
			modifiedReport.Errors = multiErr
		} else {
			modifiedReport.Errors = nil
		}
		allReports[proxy] = modifiedReport
	}
	return erroredRoutes
}

func (s *RouteReplacingSanitizer) insertFallbackListener(listeners *envoycache.Resources) {
	if listeners.Items == nil {
		listeners.Items = map[string]envoycache.Resource{}
	}

	listener := resource.NewEnvoyResource(s.fallbackListener)

	listeners.Items[listener.Self().Name] = listener
	listeners.Version += "-with-fallback-listener"
}

func (s *RouteReplacingSanitizer) insertFallbackCluster(clusters *envoycache.Resources) {
	if clusters.Items == nil {
		clusters.Items = map[string]envoycache.Resource{}
	}

	cluster := resource.NewEnvoyResource(s.fallbackCluster)

	clusters.Items[cluster.Self().Name] = cluster
	clusters.Version += "-with-fallback-cluster"
}
