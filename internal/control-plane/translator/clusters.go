package translator

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/internal/control-plane/filewatcher"
	"github.com/solo-io/gloo/internal/control-plane/reporter"
	"github.com/solo-io/gloo/internal/control-plane/translator/defaults"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

// Clusters

func (t *Translator) computeClusters(cfg *v1.Config, dependencies *pluginDependencies, endpoints endpointdiscovery.EndpointGroups) ([]*envoyapi.Cluster, []reporter.ConfigObjectReport) {
	var (
		reports  []reporter.ConfigObjectReport
		clusters []*envoyapi.Cluster
	)
	for _, upstream := range cfg.Upstreams {
		_, edsCluster := endpoints[upstream.Name]
		cluster, err := t.computeCluster(cfg, dependencies, upstream, edsCluster)
		// only append valid clusters
		if err == nil {
			clusters = append(clusters, cluster)
		}
		reports = append(reports, createReport(upstream, err))
	}
	return clusters, reports
}

func (t *Translator) computeCluster(cfg *v1.Config, dependencies *pluginDependencies, upstream *v1.Upstream, edsCluster bool) (*envoyapi.Cluster, error) {
	out := &envoyapi.Cluster{
		Name:     upstream.Name,
		Metadata: new(envoycore.Metadata),
	}
	if edsCluster {
		out.Type = envoyapi.Cluster_EDS
	}

	timeout := upstream.ConnectionTimeout
	if timeout == 0 {
		timeout = defaults.ClusterConnectionTimeout
	}
	out.ConnectTimeout = timeout

	var upstreamErrors error
	for _, plug := range t.plugins {
		upstreamPlugin, ok := plug.(plugins.UpstreamPlugin)
		if !ok {
			continue
		}
		params := &plugins.UpstreamPluginParams{
			EnvoyNameForUpstream: clusterName,
		}
		deps := dependenciesForPlugin(cfg, upstreamPlugin, dependencies)
		if deps != nil {
			params.Secrets = deps.Secrets
			params.Files = deps.Files
		}

		if err := upstreamPlugin.ProcessUpstream(params, upstream, out); err != nil {
			upstreamErrors = multierror.Append(upstreamErrors, err)
		}
	}
	if err := validateCluster(out); err != nil {
		upstreamErrors = multierror.Append(upstreamErrors, err)
	}
	return out, upstreamErrors
}

// TODO: add more validation here
func validateCluster(c *envoyapi.Cluster) error {
	if c.Type == envoyapi.Cluster_STATIC || c.Type == envoyapi.Cluster_STRICT_DNS || c.Type == envoyapi.Cluster_LOGICAL_DNS {
		if len(c.Hosts) < 1 {
			return errors.Errorf("cluster type %v specified but hosts were empty", c.Type.String())
		}
	}
	return nil
}

func dependenciesForPlugin(cfg *v1.Config, plug plugins.TranslatorPlugin, dependencies *pluginDependencies) *pluginDependencies {
	dependencyRefs := plug.GetDependencies(cfg)
	if dependencyRefs == nil {
		return nil
	}
	pluginDeps := &pluginDependencies{
		Secrets: make(secretwatcher.SecretMap),
		Files:   make(filewatcher.Files),
	}
	for _, ref := range dependencyRefs.SecretRefs {
		item, ok := dependencies.Secrets[ref]
		if ok {
			pluginDeps.Secrets[ref] = item
		}
	}
	for _, ref := range dependencyRefs.FileRefs {
		item, ok := dependencies.Files[ref]
		if ok {
			pluginDeps.Files[ref] = item
		}
	}
	return pluginDeps
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