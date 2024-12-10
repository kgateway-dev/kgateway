package proxy_syncer

import (
	"context"
	"fmt"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/translator/irtranslator"
	ggv2utils "github.com/solo-io/gloo/projects/gateway2/utils"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"go.uber.org/zap"
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
	dbg *krt.DebugHandler,
	translator *irtranslator.UpstreamTranslator,
	upstreams krt.Collection[ir.Upstream],
	uccs krt.Collection[krtcollections.UniqlyConnectedClient],
) PerClientEnvoyClusters {
	ctx = contextutils.WithLogger(ctx, "upstream-translator")
	logger := contextutils.LoggerFrom(ctx).Desugar()

	clusters := krt.NewManyCollection(upstreams, func(kctx krt.HandlerContext, up ir.Upstream) []uccWithCluster {
		logger := logger.With(zap.Stringer("upstream", up))
		uccs := krt.Fetch(kctx, uccs)
		uccWithClusterRet := make([]uccWithCluster, 0, len(uccs))

		for _, ucc := range uccs {
			logger.Debug("applying destination rules for upstream", zap.String("ucc", ucc.ResourceName()))
			func() { panic("implement CanonicalHostname") }()

			c, version := translate(kctx, ucc, ctx, translator, up)
			if c == nil {
				continue
			}
			uccWithClusterRet = append(uccWithClusterRet, uccWithCluster{
				Client:         ucc,
				Cluster:        resource.NewEnvoyResource(c),
				ClusterVersion: version,
				upstreamName:   up.ResourceName(),
			})
		}
		return uccWithClusterRet
	}, krt.WithName("PerClientEnvoyClusters"), krt.WithDebugging(dbg))
	idx := krt.NewIndex(clusters, func(ucc uccWithCluster) []string {
		return []string{ucc.Client.ResourceName()}
	})

	return PerClientEnvoyClusters{
		clusters: clusters,
		index:    idx,
	}
}

func translate(kctx krt.HandlerContext, ucc krtcollections.UniqlyConnectedClient, ctx context.Context, translator *irtranslator.UpstreamTranslator, up ir.Upstream) (*envoy_config_cluster_v3.Cluster, uint64) {

	// false here should be ok - plugins should set eds on eds clusters.
	cluster := translator.TranslateUpstream(kctx, ucc, up)
	if cluster == nil {
		return nil, 0
	}

	return cluster, ggv2utils.HashProto(cluster)
}
