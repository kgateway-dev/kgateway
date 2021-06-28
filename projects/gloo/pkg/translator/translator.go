package translator

import (
	"fmt"
	"hash/fnv"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/proto"
	"github.com/mitchellh/hashstructure"
	errors "github.com/rotisserie/eris"
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
	"go.opencensus.io/trace"
	proto2 "google.golang.org/protobuf/proto"
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
	return NewTranslatorWithHasher(sslConfigTranslator, settings, getPlugins, EnvoyCacheResourcesListToFnvHash)
}

func NewTranslatorWithHasher(
	sslConfigTranslator utils.SslConfigTranslator,
	settings *v1.Settings,
	getPlugins func() []plugins.Plugin,
	hasher func(resources []envoycache.Resource) uint64,
) Translator {
	return &translatorFactory{
		getPlugins:          getPlugins,
		settings:            settings,
		sslConfigTranslator: sslConfigTranslator,
		hasher:              hasher,
	}
}

type translatorFactory struct {
	getPlugins          func() []plugins.Plugin
	settings            *v1.Settings
	sslConfigTranslator utils.SslConfigTranslator
	hasher              func(resources []envoycache.Resource) uint64
}

func (t *translatorFactory) Translate(
	params plugins.Params,
	proxy *v1.Proxy,
) (envoycache.Snapshot, reporter.ResourceReports, *validationapi.ProxyReport, error) {
	instance := &translatorInstance{
		plugins:             t.getPlugins(),
		settings:            t.settings,
		sslConfigTranslator: t.sslConfigTranslator,
		hasher:              t.hasher,
	}
	return instance.Translate(params, proxy)
}

// a translator instance performs one
type translatorInstance struct {
	plugins             []plugins.Plugin
	settings            *v1.Settings
	sslConfigTranslator utils.SslConfigTranslator
	hasher              func(resources []envoycache.Resource) uint64
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
	clusters, clusterToUpstreamMap := t.computeClusters(params, reports, upstreamRefKeyToEndpoints, proxy)
	logger.Debugf("computing envoy endpoints for proxy: %v", proxy.Metadata.Name)

	endpoints := t.computeClusterEndpoints(params, upstreamRefKeyToEndpoints, reports)

	// Find all the EDS clusters without endpoints (can happen with kube service that have no endpoints), and create a zero sized load assignment
	// this is important as otherwise envoy will wait for them forever wondering their fate and not doing much else.
ClusterLoop:
	for _, c := range clusters {
		if c.GetType() != envoy_config_cluster_v3.Cluster_EDS {
			continue
		}
		// get upstream that generated this cluster
		upstream := clusterToUpstreamMap[c]
		endpointClusterName, err := getEndpointClusterName(c.Name, upstream)
		if err != nil {
			reports.AddError(upstream, errors.Wrapf(err, "could not marshal upstream to JSON"))
		}
		// Workaround for envoy bug: https://github.com/envoyproxy/envoy/issues/13009
		// Change the cluster eds config, forcing envoy to re-request latest EDS config
		c.EdsClusterConfig.ServiceName = endpointClusterName
		for _, ep := range endpoints {
			if ep.ClusterName == c.Name {

				// the endpoint ClusterName needs to match the cluster's EdsClusterConfig ServiceName
				ep.ClusterName = endpointClusterName
				continue ClusterLoop
			}
		}
		emptyendpointlist := &envoy_config_endpoint_v3.ClusterLoadAssignment{
			ClusterName: endpointClusterName,
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

	// run Resource Generator Plugins
	for _, plug := range t.plugins {
		resourceGeneratorPlugin, ok := plug.(plugins.ResourceGeneratorPlugin)
		if !ok {
			continue
		}
		generatedClusters, generatedEndpoints, generatedRouteConfigs, generatedListeners, err := resourceGeneratorPlugin.GeneratedResources(params, clusters, endpoints, routeConfigs, listeners)
		if err != nil {
			reports.AddError(proxy, err)
		}
		clusters = append(clusters, generatedClusters...)
		endpoints = append(endpoints, generatedEndpoints...)
		routeConfigs = append(routeConfigs, generatedRouteConfigs...)
		listeners = append(listeners, generatedListeners...)
	}

	xdsSnapshot := t.generateXDSSnapshot(clusters, endpoints, routeConfigs, listeners)

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

func (t *translatorInstance) generateXDSSnapshot(
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
	endpointsVersion := t.hasher(endpointsProto)
	clustersVersion := t.hasher(clustersProto)
	listenersVersion := t.hasher(listenersProto)

	// if clusters are updated, provider a new version of the endpoints,
	// so the clusters are warm
	return xds.NewSnapshotFromResources(
		envoycache.NewResources(fmt.Sprintf("%v-%v", clustersVersion, endpointsVersion), endpointsProto),
		envoycache.NewResources(fmt.Sprintf("%v", clustersVersion), clustersProto),
		MakeRdsResources(routeConfigs),
		envoycache.NewResources(fmt.Sprintf("%v", listenersVersion), listenersProto))
}

func EnvoyCacheResourcesListToFnvHash(resources []envoycache.Resource) uint64 {
	hasher := fnv.New64()
	// 8kb capacity, consider raising if we find the buffer is frequently being
	// re-allocated by MarshalAppend to fit larger protos.
	// the goal is to keep allocations constant for GC, without allocating an
	// unnecessarily large buffer.
	buffer := make([]byte, 0, 8*1024)
	mo := proto2.MarshalOptions{Deterministic: true}
	for _, r := range resources {
		buf := buffer[:0]
		// proto.MessageV2 will create another allocation, updating solo-kit
		// to use google protos (rather than github protos, i.e. use v2) is
		// another path to further improve performance here.
		out, err := mo.MarshalAppend(buf, proto.MessageV2(r.ResourceProto()))
		if err != nil {
			panic(errors.Wrap(err, "marshalling envoy snapshot components"))
		}
		_, err = hasher.Write(out)
		if err != nil {
			panic(errors.Wrap(err, "constructing hash for envoy snapshot components"))
		}
	}
	return hasher.Sum64()
}

// deprecated, slower than EnvoyCacheResourcesListToFnvHash
func EnvoyCacheResourcesListToHash(resources []envoycache.Resource) uint64 {
	hash, err := hashstructure.Hash(resources, nil)
	if err != nil {
		panic(errors.Wrap(err, "constructing version hash for endpoints envoy snapshot components"))
	}
	return hash
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

func getEndpointClusterName(clusterName string, upstream *v1.Upstream) (string, error) {
	hash, err := upstream.Hash(nil)
	if err != nil {
		return "", err
	}
	endpointClusterName := fmt.Sprintf("%s-%d", clusterName, hash)
	return endpointClusterName, nil
}
