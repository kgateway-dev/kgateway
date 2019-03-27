package translator

import (
	"fmt"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/mitchellh/hashstructure"
	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/reporter"
	"go.opencensus.io/trace"
)

type Translator interface {
	Translate(params plugins.Params, proxy *v1.Proxy) (envoycache.Snapshot, reporter.ResourceErrors, error)
}

type translator struct {
	plugins            []plugins.Plugin
	extensionsSettings *v1.Extensions
	settings           *v1.Settings
}

func NewTranslator(plugins []plugins.Plugin, settings *v1.Settings) Translator {
	return &translator{
		plugins:            plugins,
		extensionsSettings: settings.Extensions,
		settings:           settings,
	}
}

func (t *translator) Translate(params plugins.Params, proxy *v1.Proxy) (envoycache.Snapshot, reporter.ResourceErrors, error) {

	ctx, span := trace.StartSpan(params.Ctx, "gloo.translator.Translate")
	params.Ctx = ctx
	defer span.End()

	params.Ctx = contextutils.WithLogger(params.Ctx, "translator")
	for _, p := range t.plugins {
		if err := p.Init(plugins.InitParams{
			Ctx:                params.Ctx,
			ExtensionsSettings: t.extensionsSettings,
		}); err != nil {
			return nil, nil, errors.Wrapf(err, "plugin init failed")
		}
	}
	logger := contextutils.LoggerFrom(params.Ctx)

	resourceErrs := make(reporter.ResourceErrors)

	logger.Debugf("verifing upstream groups: %v", proxy.Metadata.Name)
	t.verifyUpstreamGroups(params, resourceErrs)

	// endpoints and listeners are shared between listeners
	logger.Debugf("computing envoy clusters for proxy: %v", proxy.Metadata.Name)
	clusters := t.computeClusters(params, resourceErrs)
	logger.Debugf("computing envoy endpoints for proxy: %v", proxy.Metadata.Name)

	endpoints := computeClusterEndpoints(params.Ctx, params.Snapshot.Upstreams.List(), params.Snapshot.Endpoints.List())

	// find all the eds clusters without endpoints (can happen with kube service that have no enpoints), and create a zero sized load assignment
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
	for _, listener := range proxy.Listeners {
		logger.Infof("computing envoy resources for listener: %v", listener.Name)
		report := func(err error, format string, args ...interface{}) {
			resourceErrs.AddError(proxy, errors.Wrapf(err, format, args...))
		}

		envoyResources := t.computeListenerResources(params, proxy, listener, report)
		if envoyResources != nil {
			routeConfigs = append(routeConfigs, envoyResources.routeConfig)
			listeners = append(listeners, envoyResources.listener)
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
			resourceErrs.AddError(proxy, err)
		}
		clusters = append(clusters, generated...)
	}

	xdsSnapshot := generateXDSSnapshot(clusters, endpoints, routeConfigs, listeners)

	return xdsSnapshot, resourceErrs, nil
}

// the set of resources returned by one iteration for a single v1.Listener
// the top level Translate function should aggregate these into a finished snapshot
type listenerResources struct {
	routeConfig *envoyapi.RouteConfiguration
	listener    *envoyapi.Listener
}

func (t *translator) computeListenerResources(params plugins.Params, proxy *v1.Proxy, listener *v1.Listener, report reportFunc) *listenerResources {
	ctx, span := trace.StartSpan(params.Ctx, "gloo.translator.Translate")
	params.Ctx = ctx
	defer span.End()

	rdsName := routeConfigName(listener)

	envoyListener := t.computeListener(params, proxy, listener, report)
	if envoyListener == nil {
		return nil
	}
	routeConfig := t.computeRouteConfig(params, proxy, listener, rdsName, report)

	return &listenerResources{
		listener:    envoyListener,
		routeConfig: routeConfig,
	}
}

func generateXDSSnapshot(clusters []*envoyapi.Cluster,
	endpoints []*envoyapi.ClusterLoadAssignment,
	routeConfigs []*envoyapi.RouteConfiguration,
	listeners []*envoyapi.Listener) envoycache.Snapshot {
	var endpointsProto, clustersProto, routesProto, listenersProto []envoycache.Resource
	for _, ep := range endpoints {
		endpointsProto = append(endpointsProto, xds.NewEnvoyResource(ep))
	}
	for _, cluster := range clusters {
		clustersProto = append(clustersProto, xds.NewEnvoyResource(cluster))
	}
	for _, routeCfg := range routeConfigs {
		// don't add empty route configs, envoy will complain
		if len(routeCfg.VirtualHosts) < 1 {
			continue
		}
		routesProto = append(routesProto, xds.NewEnvoyResource(routeCfg))
	}
	for _, listener := range listeners {
		// don't add empty listeners, envoy will complain
		if len(listener.FilterChains) < 1 {
			continue
		}
		listenersProto = append(listenersProto, xds.NewEnvoyResource(listener))
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

	return xds.NewSnapshot(fmt.Sprintf("%v", version), endpointsProto, clustersProto, routesProto, listenersProto)
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

func deduplicateEndpoints(endpoints []*envoyapi.ClusterLoadAssignment) []*envoyapi.ClusterLoadAssignment {
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
