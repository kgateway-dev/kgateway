package proxy_syncer

import (
	"context"
	"fmt"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

type uccWithCluster struct {
	Client ir.UniquelyConnectedClient
	// +krtEqualsTodo include full cluster diff in equality
	Cluster        *envoyclusterv3.Cluster
	ClusterVersion uint64
	// +krtEqualsTodo reconcile name-only equality semantics
	Name string
	// Error is the translation error for this backend/client pair, if any. Compared by message
	// in Equals because all errored clusters share one blackhole proto, so ClusterVersion can't
	// tell error states apart.
	Error error
	// BackendSource identifies the Backend this cluster was translated from, for status attribution.
	BackendSource ir.ObjectSource
	// BackendGeneration is the observed generation of the source Backend.
	BackendGeneration int64
}

func (c uccWithCluster) ResourceName() string {
	return fmt.Sprintf("%s/%s", c.Client.ResourceName(), c.Name)
}

func (c uccWithCluster) Equals(in uccWithCluster) bool {
	return c.Client.Equals(in.Client) &&
		c.ClusterVersion == in.ClusterVersion &&
		c.BackendSource == in.BackendSource &&
		c.BackendGeneration == in.BackendGeneration &&
		errString(c.Error) == errString(in.Error)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type PerClientEnvoyClusters struct {
	clusters krt.Collection[uccWithCluster]
	index    krt.Index[string, uccWithCluster]
}

func (iu *PerClientEnvoyClusters) FetchClustersForClient(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) []uccWithCluster {
	return krt.Fetch(kctx, iu.clusters, krt.FilterIndex(iu.index, ucc.ResourceName()))
}

func NewPerClientEnvoyClusters(
	ctx context.Context,
	krtopts krtutil.KrtOptions,
	translator *irtranslator.BackendTranslator,
	finalBackends krt.Collection[*ir.BackendObjectIR],
	uccs krt.Collection[ir.UniquelyConnectedClient],
) PerClientEnvoyClusters {
	translatedClusters := krt.NewManyCollection(finalBackends, func(kctx krt.HandlerContext, backendObj *ir.BackendObjectIR) []uccWithCluster {
		backendLogger := logger.With("backend", backendObj)
		uccs := krt.Fetch(kctx, uccs)
		uccWithClusterRet := make([]uccWithCluster, 0, len(uccs))

		for _, ucc := range uccs {
			backendLogger.Debug("applying destination rules for backend", "ucc", ucc.ResourceName())

			c, err := translator.TranslateBackend(ctx, kctx, ucc, backendObj)
			if c == nil {
				continue
			}
			var backendGeneration int64
			if backendObj.Obj != nil {
				backendGeneration = backendObj.Obj.GetGeneration()
			}
			uccWithClusterRet = append(uccWithClusterRet, uccWithCluster{
				Name:    c.GetName(),
				Client:  ucc,
				Cluster: c,
				// pass along the error(s) indicating to consumers that this cluster is not usable
				Error:             err,
				ClusterVersion:    utils.HashProto(c),
				BackendSource:     backendObj.GetObjectSource(),
				BackendGeneration: backendGeneration,
			})
		}
		return uccWithClusterRet
	}, krtopts.ToOptions("TranslatedPerClientEnvoyClusters")...)
	translatedIdx := krtpkg.UnnamedIndex(translatedClusters, func(ucc uccWithCluster) []string {
		return []string{ucc.Client.ResourceName()}
	})

	clusters := krt.NewManyCollection(uccs, func(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) []uccWithCluster {
		translated := krt.Fetch(kctx, translatedClusters, krt.FilterIndex(translatedIdx, ucc.ResourceName()))
		return validateTranslatedClusters(ctx, translator, translated)
	}, krtopts.ToOptions("PerClientEnvoyClusters")...)
	idx := krtpkg.UnnamedIndex(clusters, func(ucc uccWithCluster) []string {
		return []string{ucc.Client.ResourceName()}
	})

	return PerClientEnvoyClusters{
		clusters: clusters,
		index:    idx,
	}
}

func validateTranslatedClusters(
	ctx context.Context,
	translator *irtranslator.BackendTranslator,
	clusters []uccWithCluster,
) []uccWithCluster {
	if translator == nil || !translator.StrictValidationEnabled() {
		return clusters
	}

	candidates := make([]int, 0, len(clusters))
	candidateClusters := make([]*envoyclusterv3.Cluster, 0, len(clusters))
	for i, cluster := range clusters {
		if cluster.Error != nil || cluster.Cluster == nil {
			continue
		}
		candidates = append(candidates, i)
		candidateClusters = append(candidateClusters, cluster.Cluster)
	}
	if len(candidateClusters) == 0 {
		return clusters
	}

	if err := translator.ValidateClusterConfigs(ctx, candidateClusters); err == nil {
		return clusters
	} else {
		logger.Debug("strict backend batch validation failed; isolating invalid clusters", "clusters", len(candidateClusters), "error", err)
	}

	survivingClusters := make([]*envoyclusterv3.Cluster, 0, len(candidateClusters))
	survivingIndexes := make([]int, 0, len(candidateClusters))
	for _, idx := range candidates {
		if err := translator.ValidateClusterConfig(ctx, clusters[idx].Cluster); err != nil {
			markClusterValidationError(&clusters[idx], err)
			continue
		}
		survivingClusters = append(survivingClusters, clusters[idx].Cluster)
		survivingIndexes = append(survivingIndexes, idx)
	}

	if err := translator.ValidateClusterConfigs(ctx, survivingClusters); err != nil {
		logger.Error("strict backend batch validation failed after invalid cluster isolation", "clusters", len(survivingClusters), "error", err)
		for _, idx := range survivingIndexes {
			markClusterValidationError(&clusters[idx], err)
		}
	}
	return clusters
}

func markClusterValidationError(cluster *uccWithCluster, err error) {
	logger.Error("cluster failed xDS validation in strict mode", "cluster", cluster.Name, "error", err)
	cluster.Error = err
	cluster.Cluster = irtranslator.BlackholeClusterForName(cluster.Name)
	cluster.ClusterVersion = utils.HashProto(cluster.Cluster)
}
