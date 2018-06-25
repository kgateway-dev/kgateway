package translator

import (
	"fmt"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/mitchellh/hashstructure"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/internal/control-plane/filewatcher"
	"github.com/solo-io/gloo/internal/control-plane/reporter"
	"github.com/solo-io/gloo/internal/control-plane/snapshot"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/matcher"
	"github.com/solo-io/gloo/pkg/coreplugins/route-extensions"
	"github.com/solo-io/gloo/pkg/coreplugins/static"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

const (
	connMgrFilter = "envoy.http_connection_manager"
	routerFilter  = "envoy.router"
)

type Translator struct {
	plugins []plugins.TranslatorPlugin
}

// all built-in plugins should go here
var corePlugins = []plugins.TranslatorPlugin{
	&matcher.Plugin{},
	&extensions.Plugin{},
	static.NewPlugin(),
}

func NewTranslator(translatorPlugins []plugins.TranslatorPlugin) *Translator {
	translatorPlugins = append(corePlugins, translatorPlugins...)
	// special routing must be done for upstream plugins that support functions
	var functionPlugins []plugins.FunctionPlugin
	for _, plug := range translatorPlugins {
		if functionPlugin, ok := plug.(plugins.FunctionPlugin); ok {
			functionPlugins = append(functionPlugins, functionPlugin)
		}
	}

	// the route initializer plugin intializes route actions and metadata
	// including cluster weights for upstream and function destinations
	routeInitializer := newRouteInitializerPlugin()

	// the functional upstream plugins call ParseFunctionSpec on each function plugin
	// and adds the function spec to the cluster metadata
	// the functional upstream processor should be inserted at the end of the plugin chain
	// to ensure ProcessUpstream() is called before ParseFunctionSpec for each upstream
	functionalUpstreamProcessor := newFunctionalPluginProcessor(functionPlugins)

	// order matters here
	translatorPlugins = append([]plugins.TranslatorPlugin{routeInitializer}, translatorPlugins...)
	translatorPlugins = append(translatorPlugins, functionalUpstreamProcessor)

	return &Translator{
		plugins: translatorPlugins,
	}
}

func (t *Translator) Translate(role *v1.Role, inputs *snapshot.Cache) (*envoycache.Snapshot, []reporter.ConfigObjectReport) {
	log.Printf("Translation loop starting")

	var (
		clusters     []*envoyapi.Cluster
		endpoints    []*envoyapi.ClusterLoadAssignment
		routeConfigs []*envoyapi.RouteConfiguration
		listeners    []*envoyapi.Listener
	)

	// aggregate config errors by the cfg object that caused them
	configErrs := make(configErrors)

	for _, listener := range role.Listeners {
		envoyResources := t.computeListenerResources(role, listener, inputs, configErrs)
		clusters = append(clusters, envoyResources.clusters...)
		endpoints = append(endpoints, envoyResources.endpoints...)
		routeConfigs = append(routeConfigs, envoyResources.routeConfig)
		listeners = append(listeners, envoyResources.listener)
	}

	clusters = deduplicateClusters(clusters)
	endpoints = deduplicateEndpoints(endpoints)

	xdsSnapshot := generateXDSSnapshot(clusters, endpoints, routeConfigs, listeners)

	return &xdsSnapshot, configErrs.reports()
}

func (t *Translator) computeListenerResources(role *v1.Role, listener *v1.Listener, inputs *snapshot.Cache, cfgErrs configErrors) *listenerResources {
	rdsName := routeConfigName(listener)
	inputs = trimSnapshot(role, listener, inputs, cfgErrs)

	cfgErrs.initializeKeys(inputs.Cfg)

	endpoints := computeClusterEndpoints(inputs.Cfg.Upstreams, inputs.Endpoints)
	clusters := t.computeClusters(inputs, cfgErrs)
	routeConfig := t.computeRouteConfig(role, listener.Name, rdsName, inputs, cfgErrs)
	envoyListener := t.computeListener(role, listener, inputs, cfgErrs)

	return &listenerResources{
		clusters:     clusters,
		endpoints:    endpoints,
		listener:     envoyListener,
		routeConfig:  routeConfig,
		configErrors: cfgErrs,
	}
}

func generateXDSSnapshot(clusters []*envoyapi.Cluster,
	endpoints []*envoyapi.ClusterLoadAssignment,
	routeConfigs []*envoyapi.RouteConfiguration,
	listeners []*envoyapi.Listener) envoycache.Snapshot {
	var endpointsProto, clustersProto, routesProto, listenersProto []envoycache.Resource
	for _, ep := range endpoints {
		endpointsProto = append(endpointsProto, ep)
	}
	for _, cluster := range clusters {
		clustersProto = append(clustersProto, cluster)
	}
	for _, routeCfg := range routeConfigs {
		// don't add empty route configs, envoy will complain
		if len(routeCfg.VirtualHosts) < 1 {
			continue
		}
		routesProto = append(routesProto, routeCfg)
	}
	for _, listener := range listeners {
		// don't add empty listeners, envoy will complain
		if len(listener.FilterChains) < 1 {
			continue
		}
		listenersProto = append(listenersProto, listener)
	}
	// construct version
	// TODO: investigate whether we need a more sophisticated versionining algorithm
	version, err := hashstructure.Hash([][]envoycache.Resource{
		endpointsProto,
		clustersProto,
		routesProto,
		listenersProto,
	}, nil)
	if err != nil {
		panic(errors.Wrap(err, "constructing version hash for envoy snapshot components"))
	}

	return envoycache.NewSnapshot(fmt.Sprintf("%v", version), endpointsProto, clustersProto, routesProto, listenersProto)
}

// utility functions

func dependenciesForPlugin(inputs *snapshot.Cache, plug plugins.TranslatorPlugin) (secretwatcher.SecretMap, filewatcher.Files) {
	dependencyRefs := plug.GetDependencies(inputs.Cfg)
	if dependencyRefs == nil {
		return nil, nil
	}
	secrets := make(secretwatcher.SecretMap)
	files := make(filewatcher.Files)
	for _, ref := range dependencyRefs.SecretRefs {
		item, ok := inputs.Secrets[ref]
		if ok {
			secrets[ref] = item
		}
	}
	for _, ref := range dependencyRefs.FileRefs {
		item, ok := inputs.Files[ref]
		if ok {
			files[ref] = item
		}
	}
	return secrets, files
}

func deduplicateClusters(clusters []*envoyapi.Cluster) []*envoyapi.Cluster {
	mapped := make(map[string]bool)
	var deduped []*envoyapi.Cluster
	for _, c := range clusters {
		if _, added := mapped[c.Name]; added {
			continue
		}
		deduped = append(deduped, c)
	}
	return deduped
}

func deduplicateEndpoints(endpoints []*envoyapi.ClusterLoadAssignment) []*envoyapi.ClusterLoadAssignment{
	mapped := make(map[string]bool)
	var deduped []*envoyapi.ClusterLoadAssignment
	for _, ep := range endpoints {
		if _, added := mapped[ep.String()]; added {
			continue
		}
		deduped = append(deduped, ep)
	}
	return deduped
}
