package translator

import (
	"fmt"
	"time"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"go.opencensus.io/trace"
)

func (t *translator) computeClusters(params plugins.Params, reports reporter.ResourceReports) []*envoyapi.Cluster {

	ctx, span := trace.StartSpan(params.Ctx, "gloo.translator.computeClusters")
	params.Ctx = ctx
	defer span.End()

	params.Ctx = contextutils.WithLogger(params.Ctx, "compute_clusters")
	var (
		clusters []*envoyapi.Cluster
	)

	// snapshot contains both real and service-derived upstreams
	for _, upstream := range params.Snapshot.Upstreams {
		cluster := t.computeCluster(params, upstream, reports)
		clusters = append(clusters, cluster)
	}
	return clusters
}

func (t *translator) computeCluster(params plugins.Params, upstream *v1.Upstream, reports reporter.ResourceReports) *envoyapi.Cluster {
	params.Ctx = contextutils.WithLogger(params.Ctx, upstream.Metadata.Name)
	out := t.initializeCluster(upstream, params.Snapshot.Endpoints, reports)

	for _, plug := range t.plugins {
		upstreamPlugin, ok := plug.(plugins.UpstreamPlugin)
		if !ok {
			continue
		}

		if err := upstreamPlugin.ProcessUpstream(params, upstream, out); err != nil {
			reports.AddError(upstream, err)
		}
	}
	if err := validateCluster(out); err != nil {
		reports.AddError(upstream, errors.Wrapf(err, "cluster was configured improperly "+
			"by one or more plugins: %v", out))
	}
	return out
}

func (t *translator) initializeCluster(upstream *v1.Upstream, endpoints []*v1.Endpoint, reports reporter.ResourceReports) *envoyapi.Cluster {
	hcConfig, err := createHealthCheckConfig(upstream)
	if err != nil {
		reports.AddError(upstream, err)
	}
	detectCfg, err := createOutlierDetectionConfig(upstream)
	if err != nil {
		reports.AddError(upstream, err)
	}

	circuitBreakers := t.settings.GetGloo().GetCircuitBreakers()
	if circuitBreakers == nil {
		circuitBreakers = t.settings.GetCircuitBreakers()
	}

	out := &envoyapi.Cluster{
		Name:             UpstreamToClusterName(upstream.Metadata.Ref()),
		Metadata:         new(envoycore.Metadata),
		CircuitBreakers:  getCircuitBreakers(upstream.UpstreamSpec.CircuitBreakers, circuitBreakers),
		LbSubsetConfig:   createLbConfig(upstream),
		HealthChecks:     hcConfig,
		OutlierDetection: detectCfg,
		// this field can be overridden by plugins
		ConnectTimeout:       ClusterConnectionTimeout,
		Http2ProtocolOptions: getHttp2ptions(upstream.UpstreamSpec),
	}
	// set Type = EDS if we have endpoints for the upstream
	if len(endpointsForUpstream(upstream, endpoints)) > 0 {
		xds.SetEdsOnCluster(out)
	}
	return out
}

var (
	DefaultHealthCheckTimeout  = time.Second * 5
	DefaultHealthCheckInterval = time.Millisecond * 100
	DefaultThreshold           = &types.UInt32Value{
		Value: 5,
	}

	NilFieldError = func(fieldName string) error {
		return errors.Errorf("The field %s cannot be nil", fieldName)
	}
)

func createHealthCheckConfig(upstream *v1.Upstream) ([]*envoycore.HealthCheck, error) {

	if upstream.GetUpstreamSpec() == nil {
		return nil, nil
	}
	result := make([]*envoycore.HealthCheck, 0, len(upstream.GetUpstreamSpec().GetHealthChecks()))
	for i, hc := range upstream.GetUpstreamSpec().GetHealthChecks() {
		// These values are required by envoy, but not explicitly
		if hc.HealthyThreshold == nil {
			return nil, NilFieldError(fmt.Sprintf("HealthCheck[%d].HealthyThreshold", i))
		}
		if hc.UnhealthyThreshold == nil {
			return nil, NilFieldError(fmt.Sprintf("HealthCheck[%d].UnhealthyThreshold", i))
		}

		if err := hc.Validate(); err != nil {
			return nil, err
		}

		result = append(result, hc)
	}
	return result, nil
}

func createOutlierDetectionConfig(upstream *v1.Upstream) (*envoycluster.OutlierDetection, error) {
	spec := upstream.GetUpstreamSpec()
	if spec == nil {
		return nil, nil
	}
	// This should be enough validation as nothing implicitly needs to be set
	if err := spec.GetOutlierDetection().Validate(); err != nil {
		return nil, err
	}

	return spec.GetOutlierDetection(), nil
}

func createLbConfig(upstream *v1.Upstream) *envoyapi.Cluster_LbSubsetConfig {
	specGetter, ok := upstream.UpstreamSpec.UpstreamType.(v1.SubsetSpecGetter)
	if !ok {
		return nil
	}
	glooSubsetConfig := specGetter.GetSubsetSpec()
	if glooSubsetConfig == nil {
		return nil
	}

	subsetConfig := &envoyapi.Cluster_LbSubsetConfig{
		FallbackPolicy: envoyapi.Cluster_LbSubsetConfig_ANY_ENDPOINT,
	}
	for _, keys := range glooSubsetConfig.Selectors {
		subsetConfig.SubsetSelectors = append(subsetConfig.SubsetSelectors, &envoyapi.Cluster_LbSubsetConfig_LbSubsetSelector{
			Keys: keys.Keys,
		})
	}

	return subsetConfig
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

func getHttp2ptions(spec *v1.UpstreamSpec) *envoycore.Http2ProtocolOptions {
	if spec.GetUseHttp2() {
		return &envoycore.Http2ProtocolOptions{}
	}
	return nil
}
