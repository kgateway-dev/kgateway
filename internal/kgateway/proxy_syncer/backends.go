package proxy_syncer

import (
	"context"
	"fmt"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

type uccWithCluster struct {
	Client         ir.UniqlyConnectedClient
	Cluster        *envoyclusterv3.Cluster
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
	finalBackends krt.Collection[*ir.BackendObjectIR],
	uccs krt.Collection[ir.UniqlyConnectedClient],
) PerClientEnvoyClusters {
	clusters := krt.NewManyCollection(finalBackends, func(kctx krt.HandlerContext, backendObj *ir.BackendObjectIR) []uccWithCluster {
		backendLogger := logger.With("backend", backendObj)
		uccs := krt.Fetch(kctx, uccs)
		uccWithClusterRet := make([]uccWithCluster, 0, len(uccs))

		for _, ucc := range uccs {
			backendLogger.Debug("applying destination rules for backend", "ucc", ucc.ResourceName())

			c, err := translator.TranslateBackend(ctx, kctx, ucc, backendObj)
			if c == nil {
				continue
			}
			uccWithClusterRet = append(uccWithClusterRet, uccWithCluster{
				Name:    c.GetName(),
				Client:  ucc,
				Cluster: c,
				// pass along the error(s) indicating to consumers that this cluster is not usable
				Error:          err,
				ClusterVersion: utils.HashProto(c),
			})
		}
		return uccWithClusterRet
	}, krtopts.ToOptions("PerClientEnvoyClusters")...)
	idx := krtpkg.UnnamedIndex(clusters, func(ucc uccWithCluster) []string {
		return []string{ucc.Client.ResourceName()}
	})

	return PerClientEnvoyClusters{
		clusters: clusters,
		index:    idx,
	}
}
