package translator

import (
	"fmt"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/hashicorp/go-multierror"
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

type pluginDependencies struct {
	Secrets secretwatcher.SecretMap
	Files   filewatcher.Files
}

func (t *Translator) Translate(role *v1.Role, inputs *snapshot.Cache) (*envoycache.Snapshot, []reporter.ConfigObjectReport, error) {
	log.Printf("Translation loop starting")

	dependencies := &pluginDependencies{Secrets: inputs.Secrets, Files: inputs.Files}
	secrets := inputs.Secrets

	var roleErrs error

	var (
		allClusters  []*envoyapi.Cluster
		routeConfigs []*envoyapi.RouteConfiguration

		reports []reporter.ConfigObjectReport
	)

	// compute each listener independently
	for _, listener := range role.Listeners {
		virtualServices, err := virtualServicesForListener(listener, inputs.Cfg.VirtualServices)
		if err != nil {
			roleErrs = multierror.Append(roleErrs, err)
			continue
		}
		upstreams := destinationUpstreams(inputs.Cfg.Upstreams, virtualServices)
		endpoints := destinationEndpoints(upstreams, inputs.Endpoints)

		cfg := &v1.Config{
			Upstreams:       upstreams,
			VirtualServices: virtualServices,
		}

		// clusters
		clusters, upstreamReports := t.computeClusters(cfg, dependencies, endpoints)

		allClusters = append(allClusters, clusters...)
		reports = append(reports, upstreamReports...)

		// mark errored upstreams; routes that point to them are considered invalid
		errored := getErroredUpstreams(upstreamReports)

		// envoy virtual hosts
		virtualHosts, virtualServiceReports := t.computeVirtualHosts(role, cfg, errored, secrets)
		reports = append(reports, virtualServiceReports...)

		rdsName := listener.Name+"-routes"

		routeConfigs = append(routeConfigs, &envoyapi.RouteConfiguration{
			Name:         rdsName,
			VirtualHosts: virtualHosts,
		})

		// filters
		tcpFilters, err := t.constructFilters(rdsName, t.createHttpFilters())
		if err != nil {
			return nil, nil, errors.Wrapf(err, "constructing tcp filter chain for %v", listener.Name)
		}

		// finally, the listeners
		httpsListener := t.constructHttpsListener(listener.Name,
			t.config.SecurePort,
			sslFilters,
			cfg.VirtualServices,
			virtualServiceReports,
			secrets)
	}

	// endpoints
	clusterLoadAssignments := computeClusterEndpoints(inputs.Cfg.Upstreams, inputs.Endpoints)

	// proto-ify everything
	var endpointsProto []envoycache.Resource
	for _, cla := range clusterLoadAssignments {
		endpointsProto = append(endpointsProto, cla)
	}

	var clustersProto []envoycache.Resource
	for _, cluster := range deduplicateClusters(allClusters) {
		clustersProto = append(clustersProto, cluster)
	}

	var listenersProto, routesProto []envoycache.Resource

	// only add http listener and route config if we have no ssl vServices
	if len(envoyListener.FilterChains) > 0 {
		listenersProto = append(listenersProto, envoyListener)
		routesProto = append(routesProto,  )
	}

	// only add https listener and route config if we have ssl vServices
	if len(sslVirtualHosts) > 0 && len(httpsListener.FilterChains) > 0 {
		listenersProto = append(listenersProto, httpsListener)
		routesProto = append(routesProto, sslRouteConfig)
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
		return nil, nil, errors.Wrap(err, "constructing version hash for envoy snapshot components")
	}
	// construct snapshot
	xdsSnapshot := envoycache.NewSnapshot(fmt.Sprintf("%v", version), endpointsProto, clustersProto, routesProto, listenersProto)

	reports = append(reports, createReport(role, roleErrs))

	return &xdsSnapshot, reports, nil
}

func createReport(cfgObject v1.ConfigObject, err error) reporter.ConfigObjectReport {
	return reporter.ConfigObjectReport{
		CfgObject: cfgObject,
		Err:       err,
	}
}
