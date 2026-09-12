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

// gatewayStatusContributions unpacks the status half of each Gateway translation.
//
// It reads gatewayTranslationOutput directly rather than an intermediate collection of
// GatewayStatusSnapshot. That collection was a pure projection, so its only effect was to run
// GatewayStatusSnapshot.Equals -- a deep walk of every contribution of every route on the
// Gateway -- a second time per translation, on top of the one gatewayTranslationOutput.Equals
// already does. Contributions are still compared individually downstream, so status-only and
// xDS-only changes stay as isolated from each other as before.
func gatewayStatusContributions(
	outputs krt.Collection[gatewayTranslationOutput],
	krtopts krtutil.KrtOptions,
) krt.Collection[reports.StatusContribution] {
	return krt.NewManyCollection(outputs, func(_ krt.HandlerContext, output gatewayTranslationOutput) []reports.StatusContribution {
		return output.Status.Contributions
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
		// Key on the backend's own resource name, not its ObjectSource's: one Service yields a
		// BackendObjectIR per port, and ObjectSource.ResourceName() drops both the port and the
		// extra key. Two ports contributing to the same policy would then emit contributions
		// with identical KRT keys from a single collection.
		return reports.StatusContributionsFromReportMap(reports.StatusSource{
			Kind: reports.BackendPolicyStatusSource,
			Name: backend.ResourceName(),
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
