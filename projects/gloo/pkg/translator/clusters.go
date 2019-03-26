package translator

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"

	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/reporter"
	"github.com/solo-io/solo-kit/pkg/utils/contextutils"
	"go.opencensus.io/trace"
)

func (t *translator) computeClusters(params plugins.Params, resourceErrs reporter.ResourceErrors) []*envoyapi.Cluster {

	ctx, span := trace.StartSpan(params.Ctx, "gloo.translator.computeClusters")
	params.Ctx = ctx
	defer span.End()

	params.Ctx = contextutils.WithLogger(params.Ctx, "compute_clusters")
	var (
		clusters []*envoyapi.Cluster
	)
	for _, upstream := range params.Snapshot.Upstreams.List() {
		cluster := t.computeCluster(params, upstream, resourceErrs)
		clusters = append(clusters, cluster)
	}
	return clusters
}

func (t *translator) computeCluster(params plugins.Params, upstream *v1.Upstream, resourceErrs reporter.ResourceErrors) *envoyapi.Cluster {
	params.Ctx = contextutils.WithLogger(params.Ctx, upstream.Metadata.Name)
	out := t.initializeCluster(upstream, params.Snapshot.Endpoints.List())

	for _, plug := range t.plugins {
		upstreamPlugin, ok := plug.(plugins.UpstreamPlugin)
		if !ok {
			continue
		}

		if err := upstreamPlugin.ProcessUpstream(params, upstream, out); err != nil {
			resourceErrs.AddError(upstream, err)
		}
	}
	if err := validateCluster(out); err != nil {
		resourceErrs.AddError(upstream, errors.Wrapf(err, "cluster was configured improperly "+
			"by one or more plugins: %v", out))
	}
	return out
}

func (t *translator) initializeCluster(upstream *v1.Upstream, endpoints []*v1.Endpoint) *envoyapi.Cluster {
	out := &envoyapi.Cluster{
		Name:            UpstreamToClusterName(upstream.Metadata.Ref()),
		Metadata:        new(envoycore.Metadata),
		CircuitBreakers: getCircuitBreakers(upstream.UpstreamSpec.CircuitBreakers, t.settings.CircuitBreakers),
		// this field can be overridden by plugins
		ConnectTimeout: ClusterConnectionTimeout,
	}
	// set Type = EDS if we have endpoints for the upstream
	if len(endpointsForUpstream(upstream, endpoints)) > 0 {
		out.ClusterDiscoveryType = &envoyapi.Cluster_Type{
			Type: envoyapi.Cluster_EDS,
		}
	}
	return out
}

// TODO: add more validation here
func validateCluster(c *envoyapi.Cluster) error {
	if c.GetClusterType() != nil {
		// TODO(yuval-k): this is a custom cluster, we cant validate it for now.
		return nil
	}
	clusterType := c.GetType()
	if clusterType == envoyapi.Cluster_STATIC || clusterType == envoyapi.Cluster_STRICT_DNS || clusterType == envoyapi.Cluster_LOGICAL_DNS {
		if len(c.Hosts) == 0 && (c.LoadAssignment == nil || len(c.LoadAssignment.Endpoints) == 0) {
			return errors.Errorf("cluster type %v specified but LoadAssignment was empty", clusterType.String())
		}
	}
	return nil
}

// Convert the first non nil circuit breaker.
func getCircuitBreakers(cfgs ...*v1.CircuitBreakerConfig) *envoycluster.CircuitBreakers {
	for _, cfg := range cfgs {
		if cfg != nil {
			envoyCfg := &envoycluster.CircuitBreakers{}
			envoyCfg.Thresholds = []*envoycluster.CircuitBreakers_Thresholds{{
				MaxConnections:     cfg.MaxConnections,
				MaxPendingRequests: cfg.MaxPendingRequests,
				MaxRequests:        cfg.MaxRequests,
				MaxRetries:         cfg.MaxRetries,
			}}
			return envoyCfg
		}
	}
	return nil
}
