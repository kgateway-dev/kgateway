package translator

import (
	"fmt"

	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/mitchellh/hashstructure"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"go.opencensus.io/trace"
)

type Translator interface {
	Translate(params plugins.Params, proxy *v1.Proxy) (envoycache.Snapshot, reporter.ResourceReports, *validationapi.ProxyReport, error)
}

func NewTranslator(sslConfigTranslator utils.SslConfigTranslator, settings *v1.Settings, getPlugins func() []plugins.Plugin) Translator {
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

func (t *translatorFactory) Translate(params plugins.Params, proxy *v1.Proxy) (envoycache.Snapshot, reporter.ResourceReports, *validationapi.ProxyReport, error) {
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

func (t *translatorInstance) Translate(params plugins.Params, proxy *v1.Proxy) (envoycache.Snapshot, reporter.ResourceReports, *validationapi.ProxyReport, error) {

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

	// endpoints and listeners are shared between listeners
	logger.Debugf("computing envoy clusters for proxy: %v", proxy.Metadata.Name)
	clusters := t.computeClusters(params, reports)
	logger.Debugf("computing envoy endpoints for proxy: %v", proxy.Metadata.Name)

	endpoints := computeClusterEndpoints(params.Ctx, params.Snapshot.Upstreams, params.Snapshot.Endpoints)

	// Find all the EDS clusters without endpoints (can happen with kube service that have no endpoints), and create a zero sized load assignment
	// this is important as otherwise envoy will wait for them forever wondering their fate and not doing much else.
ClusterLoop:
	for _, c := range clusters {
		if c.GetType() != envoyapi.Cluster_EDS {
			continue
		}
		for _, ep := range endpoints {
			if ep.ClusterName == c.Name {
				continue ClusterLoop
			}
		}
		emptyendpointlist := &envoyapi.ClusterLoadAssignment{
			ClusterName: c.Name,
		}

		endpoints = append(endpoints, emptyendpointlist)
	}

	var (
		routeConfigs []*envoyapi.RouteConfiguration
		listeners    []*envoyapi.Listener
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
	routeConfig *envoyapi.RouteConfiguration
	listener    *envoyapi.Listener
}

func (t *translatorInstance) computeListenerResources(params plugins.Params, proxy *v1.Proxy, listener *v1.Listener, listenerReport *validationapi.ListenerReport) *listenerResources {
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

func generateXDSSnapshot(clusters []*envoyapi.Cluster,
	endpoints []*envoyapi.ClusterLoadAssignment,
	routeConfigs []*envoyapi.RouteConfiguration,
	listeners []*envoyapi.Listener) envoycache.Snapshot {

	var endpointsProto, clustersProto, listenersProto []envoycache.Resource

	for _, ep := range endpoints {
		endpointsProto = append(endpointsProto, xds.NewEnvoyResource(ep))
	}
	for _, cluster := range clusters {
		clustersProto = append(clustersProto, xds.NewEnvoyResource(cluster))
	}
	for _, listener := range listeners {
		// don't add empty listeners, envoy will complain
		if len(listener.FilterChains) < 1 {
			continue
		}
		listenersProto = append(listenersProto, xds.NewEnvoyResource(listener))
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

func MakeRdsResources(routeConfigs []*envoyapi.RouteConfiguration) envoycache.Resources {
	var routesProto []envoycache.Resource

	for _, routeCfg := range routeConfigs {
		// don't add empty route configs, envoy will complain
		if len(routeCfg.VirtualHosts) < 1 {
			continue
		}
		routesProto = append(routesProto, xds.NewEnvoyResource(routeCfg))
	}

	routesVersion, err := hashstructure.Hash(routesProto, nil)
	if err != nil {
		panic(errors.Wrap(err, "constructing version hash for routes envoy snapshot components"))
	}
	return envoycache.NewResources(fmt.Sprintf("%v", routesVersion), routesProto)
}
