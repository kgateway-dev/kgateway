package proxy_syncer

import (
	"context"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"google.golang.org/protobuf/proto"
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
	// resourceName caches the KRT identity key, which KRT recomputes for every row on
	// every event. Rows are per-client x per-backend, so computing it on each call is a
	// per-call allocation multiplied by both dimensions.
	// +noKrtEquals derived from Client and Name; a difference makes it a different KRT row
	resourceName string
}

func (c uccWithCluster) ResourceName() string {
	// Fall back for rows built as bare struct literals (tests) that skip the cache.
	if c.resourceName == "" {
		return uccClusterResourceName(c.Client, c.Name)
	}
	return c.resourceName
}

func uccClusterResourceName(client ir.UniquelyConnectedClient, name string) string {
	return client.ResourceName() + "/" + name
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
	clusters := krt.NewManyCollection(finalBackends, func(kctx krt.HandlerContext, backendObj *ir.BackendObjectIR) []uccWithCluster {
		backendLogger := logger.With("backend", backendObj)
		clients := krt.Fetch(kctx, uccs)
		if len(clients) == 0 {
			return nil
		}
		base := translator.TranslateBackendBase(ctx, backendObj)
		if base == nil {
			return nil
		}
		uccWithClusterRet := make([]uccWithCluster, 0, len(clients))

		for _, ucc := range clients {
			backendLogger.Debug("applying destination rules for backend", "ucc", ucc.ResourceName())

			c, err := translator.ApplyPerClient(kctx, ctx, ucc, backendObj, base)
			if c == nil {
				// Keep the existing dense ownership model for this preparatory
				// change: every KRT row receives its own proto even when no overlay
				// applies. A later change can make sharing explicit and enforce
				// immutability at the collection boundary.
				c = proto.Clone(base.Cluster).(*envoyclusterv3.Cluster)
				if err == nil {
					err = base.Error
				}
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
				resourceName:      uccClusterResourceName(ucc, c.GetName()),
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
