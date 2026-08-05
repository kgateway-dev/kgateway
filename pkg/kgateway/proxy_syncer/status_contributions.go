package proxy_syncer

import (
	"strconv"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

func gatewayStatusContributions(
	snapshots krt.Collection[GatewayStatusSnapshot],
	krtopts krtutil.KrtOptions,
) krt.Collection[reports.StatusContribution] {
	return krt.NewManyCollection(snapshots, func(_ krt.HandlerContext, snapshot GatewayStatusSnapshot) []reports.StatusContribution {
		return snapshot.Contributions
	}, krtopts.ToOptions("GatewayStatusContributions")...)
}

func backendPolicyStatusContributions(
	backends krt.Collection[*ir.BackendObjectIR],
	excludedPolicyKinds map[schema.GroupKind]struct{},
	krtopts krtutil.KrtOptions,
) krt.Collection[reports.StatusContribution] {
	return krt.NewManyCollection(backends, func(_ krt.HandlerContext, backend *ir.BackendObjectIR) []reports.StatusContribution {
		if backend == nil {
			return nil
		}
		reportMap := GenerateBackendPolicyReport([]*ir.BackendObjectIR{backend}, excludedPolicyKinds)
		return reports.StatusContributionsFromReportMap(reports.StatusSource{
			Kind: reports.BackendPolicyStatusSource,
			Name: backend.GetObjectSource().ResourceName(),
		}, reportMap)
	}, krtopts.ToOptions("BackendPolicyStatusContributions")...)
}

func backendStatusContributions(
	backends krt.Collection[ir.BackendObjectIR],
	clusters krt.Collection[uccWithCluster],
	extraConditions krt.Collection[ir.BackendObjectStatus],
	krtopts krtutil.KrtOptions,
) krt.Collection[reports.StatusContribution] {
	clusterByBackendGeneration := krtpkg.UnnamedIndex(clusters, func(cluster uccWithCluster) []string {
		if cluster.BackendSource.GetGroupKind() != wellknown.BackendGVK.GroupKind() {
			return nil
		}
		return []string{backendGenerationKey(cluster.BackendSource.ResourceName(), cluster.BackendGeneration)}
	})
	extraByBackend := krtpkg.UnnamedIndex(extraConditions, func(status ir.BackendObjectStatus) []string {
		return []string{status.Source.ResourceName()}
	})

	return krt.NewCollection(backends, func(kctx krt.HandlerContext, backend ir.BackendObjectIR) *reports.StatusContribution {
		if backend.Obj == nil {
			return nil
		}
		resourceName := backend.GetObjectSource().ResourceName()
		matchingClusters := krt.Fetch(kctx, clusters, krt.FilterIndex(
			clusterByBackendGeneration,
			backendGenerationKey(resourceName, backend.Obj.GetGeneration()),
		))
		matchingExtra := krt.Fetch(kctx, extraConditions, krt.FilterIndex(extraByBackend, resourceName))
		reportMap := GenerateBackendStatusReport([]ir.BackendObjectIR{backend}, matchingClusters, matchingExtra)
		contributions := reports.StatusContributionsFromReportMap(reports.StatusSource{
			Kind: reports.BackendStatusSource,
			Name: resourceName,
		}, reportMap)
		if len(contributions) == 0 {
			return nil
		}
		return &contributions[0]
	}, krtopts.ToOptions("BackendStatusContributions")...)
}

func backendGenerationKey(resourceName string, generation int64) string {
	return resourceName + "@" + strconv.FormatInt(generation, 10)
}
