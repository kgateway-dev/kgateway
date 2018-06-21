package translator

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/internal/control-plane/snapshot"
	"github.com/solo-io/gloo/internal/control-plane/translator/defaults"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/plugins"
)

func (t *Translator) computeClusters(inputs *snapshot.Cache, configErrs configErrors) []*envoyapi.Cluster {
	var (
		clusters []*envoyapi.Cluster
	)
	for _, upstream := range inputs.Cfg.Upstreams {
		cluster, err := t.computeCluster(inputs, upstream)
		if err != nil {
			configErrs[upstream] = multierror.Append(configErrs[upstream], err)
			continue
		}
		// only append valid clusters
		clusters = append(clusters, cluster)
	}
	return clusters
}

func (t *Translator) computeCluster(inputs *snapshot.Cache, upstream *v1.Upstream) (*envoyapi.Cluster, error) {
	out := initializeCluster(upstream, inputs.Endpoints)

	var upstreamErrors error
	for _, plug := range t.plugins {
		upstreamPlugin, ok := plug.(plugins.UpstreamPlugin)
		if !ok {
			continue
		}
		secrets, files := dependenciesForPlugin(inputs, upstreamPlugin)
		params := &plugins.UpstreamPluginParams{
			EnvoyNameForUpstream: clusterName,
			Secrets: secrets,
			Files: files,
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

func initializeCluster(upstream *v1.Upstream, endpoints endpointdiscovery.EndpointGroups) *envoyapi.Cluster {
	out := &envoyapi.Cluster{
		Name:     upstream.Name,
		Metadata: new(envoycore.Metadata),
	}
	// set Type = EDS if we have endpoints for the upstream
	if _, edsCluster := endpoints[upstream.Name]; edsCluster {
		out.Type = envoyapi.Cluster_EDS
	}
	// set connection timeout
	timeout := upstream.ConnectionTimeout
	if timeout == 0 {
		timeout = defaults.ClusterConnectionTimeout
	}
	out.ConnectTimeout = timeout
	return out
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
