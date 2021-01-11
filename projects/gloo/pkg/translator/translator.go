package translator

import (
	"fmt"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/proto"
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	"github.com/mitchellh/hashstructure"
	errors "github.com/rotisserie/eris"
	"go.opencensus.io/trace"
)

type Translator interface {
	Translate(
		params plugins.Params,
		proxy *v1.Proxy,
	) (envoycache.Snapshot, reporter.ResourceReports, *validationapi.ProxyReport, error)
}

func NewTranslator(
	sslConfigTranslator utils.SslConfigTranslator,
	settings *v1.Settings,
	getPlugins func() []plugins.Plugin,
) Translator {
	return &translatorFactory{
		getPlugins:          getPlugins,
		settings:            settings,
		sslConfigTranslator: sslConfigTranslator,
	}
}

type translatorFactory struct {
	getPlugins          func() []plugins.Plugin
	settings            *v1.Settings
	sslConfigTranslator utils.SslConfigTranslator
}

func (t *translatorFactory) Translate(
	params plugins.Params,
	proxy *v1.Proxy,
) (envoycache.Snapshot, reporter.ResourceReports, *validationapi.ProxyReport, error) {
	instance := &translatorInstance{
		plugins:             t.getPlugins(),
		settings:            t.settings,
		sslConfigTranslator: t.sslConfigTranslator,
	}
	return instance.Translate(params, proxy)
}

// a translator instance performs one
type translatorInstance struct {
	plugins             []plugins.Plugin
	settings            *v1.Settings
	sslConfigTranslator utils.SslConfigTranslator
}

func (t *translatorInstance) Translate(
	params plugins.Params,
	proxy *v1.Proxy,
) (envoycache.Snapshot, reporter.ResourceReports, *validationapi.ProxyReport, error) {

	ctx, span := trace.StartSpan(params.Ctx, "gloo.translator.Translate")
	params.Ctx = ctx
	defer span.End()

	params.Ctx = contextutils.WithLogger(params.Ctx, "translator")
	for _, p := range t.plugins {
		if err := p.Init(plugins.InitParams{
			Ctx:      params.Ctx,
			Settings: t.settings,
		}); err != nil {
			return nil, nil, nil, errors.Wrapf(err, "plugin init failed")
		}
	}
	logger := contextutils.LoggerFrom(params.Ctx)

	reports := make(reporter.ResourceReports)

	logger.Debugf("verifying upstream groups: %v", proxy.Metadata.Name)
	t.verifyUpstreamGroups(params, reports)

	upstreamRefKeyToEndpoints := createUpstreamToEndpointsMap(params.Snapshot.Upstreams, params.Snapshot.Endpoints)

	// endpoints and listeners are shared between listeners
	logger.Debugf("computing envoy clusters for proxy: %v", proxy.Metadata.Name)
	clusters := t.computeClusters(params, reports, upstreamRefKeyToEndpoints, proxy)
	logger.Debugf("computing envoy endpoints for proxy: %v", proxy.Metadata.Name)

	endpoints := t.computeClusterEndpoints(params, upstreamRefKeyToEndpoints, reports)

	// Find all the EDS clusters without endpoints (can happen with kube service that have no endpoints), and create a zero sized load assignment
	// this is important as otherwise envoy will wait for them forever wondering their fate and not doing much else.
ClusterLoop:
	for _, c := range clusters {
		if c.GetType() != envoy_config_cluster_v3.Cluster_EDS {
			continue
		}
		for _, ep := range endpoints {
			if ep.ClusterName == c.Name {
				continue ClusterLoop
			}
		}
		emptyendpointlist := &envoy_config_endpoint_v3.ClusterLoadAssignment{
			ClusterName: c.Name,
		}
		// make sure to call EndpointPlugin with empty endpoint
		for _, upstream := range params.Snapshot.Upstreams {
			if UpstreamToClusterName(&core.ResourceRef{
				Name:      upstream.Metadata.Name,
				Namespace: upstream.Metadata.Namespace,
			}) == c.Name {
				for _, plugin := range t.plugins {
					ep, ok := plugin.(plugins.EndpointPlugin)
					if ok {
						if err := ep.ProcessEndpoints(params, upstream, emptyendpointlist); err != nil {
							reports.AddError(upstream, err)
						}
					}
				}
			}
		}

		endpoints = append(endpoints, emptyendpointlist)
	}

	var (
		routeConfigs []*envoy_config_route_v3.RouteConfiguration
		listeners    []*envoy_config_listener_v3.Listener
	)

	proxyRpt := validation.MakeReport(proxy)

	for i, listener := range proxy.Listeners {
		listenerReport := proxyRpt.ListenerReports[i]

		logger.Infof("computing envoy resources for listener: %v", listener.Name)

		envoyResources := t.computeListenerResources(params, proxy, listener, listenerReport)
		if envoyResources != nil {
			listeners = append(listeners, envoyResources.listener)
			if envoyResources.routeConfig != nil {
				routeConfigs = append(routeConfigs, envoyResources.routeConfig)
			}
		}
	}

	// run Cluster Generator Plugins
	for _, plug := range t.plugins {
		clusterGeneratorPlugin, ok := plug.(plugins.ClusterGeneratorPlugin)
		if !ok {
			continue
		}
		generated, err := clusterGeneratorPlugin.GeneratedClusters(params)
		if err != nil {
			reports.AddError(proxy, err)
		}
		clusters = append(clusters, generated...)
	}

	xdsSnapshot := generateXDSSnapshot(clusters, endpoints, routeConfigs, listeners)

	if err := validation.GetProxyError(proxyRpt); err != nil {
		reports.AddError(proxy, err)
	}

	// TODO: add a settings flag to allow accepting proxy on warnings
	if warnings := validation.GetProxyWarning(proxyRpt); len(warnings) > 0 {
		for _, warning := range warnings {
			reports.AddWarning(proxy, warning)
		}
	}

	return xdsSnapshot, reports, proxyRpt, nil
}

// the set of resources returned by one iteration for a single v1.Listener
// the top level Translate function should aggregate these into a finished snapshot
type listenerResources struct {
	routeConfig *envoy_config_route_v3.RouteConfiguration
	listener    *envoy_config_listener_v3.Listener
}

func (t *translatorInstance) computeListenerResources(
	params plugins.Params,
	proxy *v1.Proxy,
	listener *v1.Listener,
	listenerReport *validationapi.ListenerReport,
) *listenerResources {
	ctx, span := trace.StartSpan(params.Ctx, "gloo.translator.Translate")
	params.Ctx = ctx
	defer span.End()

	rdsName := routeConfigName(listener)

	// Calculate routes before listeners, so that HttpFilters is called after ProcessVirtualHost\ProcessRoute
	routeConfig := t.computeRouteConfig(params, proxy, listener, rdsName, listenerReport)

	envoyListener := t.computeListener(params, proxy, listener, listenerReport)
	if envoyListener == nil {
		return nil
	}

	return &listenerResources{
		listener:    envoyListener,
		routeConfig: routeConfig,
	}
}

func generateXDSSnapshot(
	clusters []*envoy_config_cluster_v3.Cluster,
	endpoints []*envoy_config_endpoint_v3.ClusterLoadAssignment,
	routeConfigs []*envoy_config_route_v3.RouteConfiguration,
	listeners []*envoy_config_listener_v3.Listener,
) envoycache.Snapshot {

	var endpointsProto, clustersProto, listenersProto []envoycache.Resource

	for _, ep := range endpoints {
		endpointsProto = append(endpointsProto, resource.NewEnvoyResource(proto.Clone(ep)))
	}
	for _, cluster := range clusters {
		clustersProto = append(clustersProto, resource.NewEnvoyResource(proto.Clone(cluster)))
	}
	for _, listener := range listeners {
		// don't add empty listeners, envoy will complain
		if len(listener.FilterChains) < 1 {
			continue
		}
		listenersProto = append(listenersProto, resource.NewEnvoyResource(proto.Clone(listener)))
	}
	// construct version
	// TODO: investigate whether we need a more sophisticated versioning algorithm
	endpointsVersion, err := hashstructure.Hash(endpointsProto, nil)
	if err != nil {
		panic(errors.Wrap(err, "constructing version hash for endpoints envoy snapshot components"))
	}

	clustersVersion, err := hashstructure.Hash(clustersProto, nil)
	if err != nil {
		panic(errors.Wrap(err, "constructing version hash for clusters envoy snapshot components"))
	}

	listenersVersion, err := hashstructure.Hash(listenersProto, nil)
	if err != nil {
		panic(errors.Wrap(err, "constructing version hash for listeners envoy snapshot components"))
	}

	// if clusters are updated, provider a new version of the endpoints,
	// so the clusters are warm
	return xds.NewSnapshotFromResources(
		envoycache.NewResources(fmt.Sprintf("%v-%v", clustersVersion, endpointsVersion), endpointsProto),
		envoycache.NewResources(fmt.Sprintf("%v", clustersVersion), clustersProto),
		MakeRdsResources(routeConfigs),
		envoycache.NewResources(fmt.Sprintf("%v", listenersVersion), listenersProto))

}

func MakeRdsResources(routeConfigs []*envoy_config_route_v3.RouteConfiguration) envoycache.Resources {
	var routesProto []envoycache.Resource

	for _, routeCfg := range routeConfigs {
		// don't add empty route configs, envoy will complain
		if len(routeCfg.VirtualHosts) < 1 {
			continue
		}
		routesProto = append(routesProto, resource.NewEnvoyResource(proto.Clone(routeCfg)))
	}

	routesVersion, err := hashstructure.Hash(routesProto, nil)
	if err != nil {
		panic(errors.Wrap(err, "constructing version hash for routes envoy snapshot components"))
	}
	return envoycache.NewResources(fmt.Sprintf("%v", routesVersion), routesProto)
}
