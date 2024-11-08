package proxy_syncer

import (
	"context"
	"fmt"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	ggv2utils "github.com/solo-io/gloo/projects/gateway2/utils"
	cluster "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/cluster"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
)

type uccWithCluster struct {
	Client         krtcollections.UniqlyConnectedClient
	Cluster        envoycache.Resource
	ClusterVersion uint64
	upstreamName   string
}

func (c uccWithCluster) ResourceName() string {
	return fmt.Sprintf("%s/%s", c.Client.ResourceName(), c.upstreamName)
}

func (c uccWithCluster) Equals(in uccWithCluster) bool {
	return c.Client.Equals(in.Client) && c.ClusterVersion == in.ClusterVersion
}

type PerClientEnvoyClusters struct {
	clusters krt.Collection[uccWithCluster]
	index    krt.Index[string, uccWithCluster]
}

func (iu *PerClientEnvoyClusters) FetchClustersForClient(kctx krt.HandlerContext, ucc krtcollections.UniqlyConnectedClient) []uccWithCluster {
	return krt.Fetch(kctx, iu.clusters, krt.FilterIndex(iu.index, ucc.ResourceName()))
}

func NewPerClientEnvoyClusters(
	ctx context.Context,
	translator setup.TranslatorFactory,
	upstreams krt.Collection[krtcollections.UpstreamWrapper],
	uccs krt.Collection[krtcollections.UniqlyConnectedClient],
	ks krt.Collection[krtcollections.ResourceWrapper[*gloov1.Secret]],
	settings krt.Singleton[glookubev1.Settings],
	destinationRulesIndex DestinationRuleIndex,
) PerClientEnvoyClusters {
	ctx = contextutils.WithLogger(ctx, "upstream-translator")
	logger := contextutils.LoggerFrom(ctx).Desugar()

	clusters := krt.NewManyCollection(upstreams, func(kctx krt.HandlerContext, up krtcollections.UpstreamWrapper) []uccWithCluster {
		logger := logger.With(zap.Stringer("upstream", up))
		uccs := krt.Fetch(kctx, uccs)
		uccWithClusterRet := make([]uccWithCluster, 0, len(uccs))
		secrets := krt.Fetch(kctx, ks)
		ksettings := krt.FetchOne(kctx, settings.AsCollection())
		settings := &ksettings.Spec

		for _, ucc := range uccs {
			logger.Debug("applying destination rules for upstream", zap.String("ucc", ucc.ResourceName()))

			hostname := ggv2utils.GetHostnameForUpstream(up.Inner)

			destrule := destinationRulesIndex.FetchDestRulesFor(kctx, ucc.Namespace, hostname, ucc.Labels)
			if destrule != nil {
				logger.Debug("found destination rule", zap.Stringer("destrule", destrule), zap.String("targetHost", hostname))
			}

			// if dest rules applies, translation will give the upstream a different name
			// as the usptream hash will be different.
			// save the original name so we can set it as the ServiceName on the cluster;
			// this ensures that the name will match the ClusterLoadAssignment ClusterName.
			upstream, name := ApplyDestRulesForUpstream(destrule, up.Inner)

			latestSnap := &gloosnapshot.ApiSnapshot{}
			latestSnap.Secrets = make([]*gloov1.Secret, 0, len(secrets))
			for _, s := range secrets {
				latestSnap.Secrets = append(latestSnap.Secrets, s.Inner)
			}

			c, version := translate(ctx, settings, translator, latestSnap, upstream)
			if c == nil {
				continue
			}
			if name != "" && c.GetEdsClusterConfig() != nil {
				c.GetEdsClusterConfig().ServiceName = name
			}

			uccWithClusterRet = append(uccWithClusterRet, uccWithCluster{
				Client:         ucc,
				Cluster:        resource.NewEnvoyResource(c),
				ClusterVersion: version,
				upstreamName:   up.ResourceName(),
			})
		}
		return uccWithClusterRet
	})
	idx := krt.NewIndex(clusters, func(ucc uccWithCluster) []string {
		return []string{ucc.Client.ResourceName()}
	})

	return PerClientEnvoyClusters{
		clusters: clusters,
		index:    idx,
	}
}

func translate(ctx context.Context, settings *gloov1.Settings, translator setup.TranslatorFactory, snap *gloosnapshot.ApiSnapshot, up *gloov1.Upstream) (*envoy_config_cluster_v3.Cluster, uint64) {
	ctx = settingsutil.WithSettings(ctx, settings)

	params := plugins.Params{
		Ctx:      ctx,
		Settings: settings,
		Snapshot: snap,
		Messages: map[*core.ResourceRef][]string{},
	}

	// false here should be ok - plugins should set eds on eds clusters.
	cluster, _ := translator.NewClusterTranslator(ctx, settings).TranslateCluster(params, up)
	if cluster == nil {
		return nil, 0
	}

	return cluster, ggv2utils.HashProto(cluster)
}

func ApplyDestRulesForUpstream(destrule *DestinationRuleWrapper, u *gloov1.Upstream) (*gloov1.Upstream, string) {
	if destrule != nil {
		trafficPolicy := getTraficPolicy(destrule, ggv2utils.GetPortForUpstream(u))
		if outlier := trafficPolicy.GetOutlierDetection(); outlier != nil {
			name := krtcollections.GetEndpointClusterName(u)

			// do not mutate the original upstream
			up := *u

			if getLocalityLbSetting(trafficPolicy) != nil {
				if up.GetLoadBalancerConfig() == nil {
					up.LoadBalancerConfig = &gloov1.LoadBalancerConfig{}
				}
				up.GetLoadBalancerConfig().LocalityConfig = &gloov1.LoadBalancerConfig_LocalityWeightedLbConfig{}
			}
			out := &cluster.OutlierDetection{
				Consecutive_5Xx:  outlier.GetConsecutive_5XxErrors(),
				Interval:         outlier.GetInterval(),
				BaseEjectionTime: outlier.GetBaseEjectionTime(),
			}
			if e := outlier.GetConsecutiveGatewayErrors(); e != nil {
				v := e.GetValue()
				out.ConsecutiveGatewayFailure = &wrapperspb.UInt32Value{Value: v}
				if v > 0 {
					v = 100
				}
				out.EnforcingConsecutiveGatewayFailure = &wrapperspb.UInt32Value{Value: v}
			}
			if outlier.GetMaxEjectionPercent() > 0 {
				out.MaxEjectionPercent = &wrapperspb.UInt32Value{Value: uint32(outlier.GetMaxEjectionPercent())}
			}
			if outlier.GetSplitExternalLocalOriginErrors() {
				out.SplitExternalLocalOriginErrors = true
				if outlier.GetConsecutiveLocalOriginFailures().GetValue() > 0 {
					out.ConsecutiveLocalOriginFailure = &wrapperspb.UInt32Value{Value: outlier.GetConsecutiveLocalOriginFailures().Value}
					out.EnforcingConsecutiveLocalOriginFailure = &wrapperspb.UInt32Value{Value: 100}
				}
				// SuccessRate based outlier detection should be disabled.
				out.EnforcingLocalOriginSuccessRate = &wrapperspb.UInt32Value{Value: 0}
			}
			minHealthPercent := outlier.GetMinHealthPercent()
			if minHealthPercent >= 0 {
				if up.GetLoadBalancerConfig() == nil {
					up.LoadBalancerConfig = &gloov1.LoadBalancerConfig{}
				}
				up.GetLoadBalancerConfig().HealthyPanicThreshold = wrapperspb.Double(float64(minHealthPercent))
			}

			up.OutlierDetection = out

			return &up, name
		}
	}

	return u, ""
}
