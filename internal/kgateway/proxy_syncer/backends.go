package proxy_syncer

import (
	"context"
	"fmt"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	ggv2utils "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
)

type uccWithCluster struct {
	Client         ir.UniqlyConnectedClient
	Cluster        *envoy_config_cluster_v3.Cluster
	ClusterVersion uint64
	Name           string
	Error          error
}

func (c uccWithCluster) ResourceName() string {
	return fmt.Sprintf("%s/%s", c.Client.ResourceName(), c.Name)
}

func (c uccWithCluster) Equals(in uccWithCluster) bool {
	return c.Client.Equals(in.Client) && c.ClusterVersion == in.ClusterVersion
}

type PerClientEnvoyClusters struct {
	clusters krt.Collection[uccWithCluster]
	index    krt.Index[string, uccWithCluster]
}

func (iu *PerClientEnvoyClusters) FetchClustersForClient(kctx krt.HandlerContext, ucc ir.UniqlyConnectedClient) []uccWithCluster {
	return krt.Fetch(kctx, iu.clusters, krt.FilterIndex(iu.index, ucc.ResourceName()))
}

func NewPerClientEnvoyClusters(
	ctx context.Context,
	krtopts krtutil.KrtOptions,
	translator *irtranslator.BackendTranslator,
	backendObjs krt.Collection[ir.BackendObjectIR],
	uccs krt.Collection[ir.UniqlyConnectedClient],
) PerClientEnvoyClusters {
	ctx = contextutils.WithLogger(ctx, "backend-translator")
	logger := contextutils.LoggerFrom(ctx).Desugar()

	clusters := krt.NewManyCollection(backendObjs, func(kctx krt.HandlerContext, backendObj ir.BackendObjectIR) []uccWithCluster {
		logger := logger.With(zap.Stringer("backend", backendObj))
		uccs := krt.Fetch(kctx, uccs)
		uccWithClusterRet := make([]uccWithCluster, 0, len(uccs))

		for _, ucc := range uccs {
			logger.Debug("applying destination rules for backend", zap.String("ucc", ucc.ResourceName()))

			c, err := translator.TranslateBackend(kctx, ucc, backendObj)
			if c == nil {
				continue
			}
			uccWithClusterRet = append(uccWithClusterRet, uccWithCluster{
				Client:         ucc,
				Cluster:        c,
				Name:           c.GetName(),
				Error:          err,
				ClusterVersion: ggv2utils.HashProto(c),
			})
		}
		return uccWithClusterRet
	}, krtopts.ToOptions("PerClientEnvoyClusters")...)
	idx := krt.NewIndex(clusters, func(ucc uccWithCluster) []string {
		return []string{ucc.Client.ResourceName()}
	})

	return PerClientEnvoyClusters{
		clusters: clusters,
		index:    idx,
	}
}
