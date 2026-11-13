package proxy_syncer

import (
	"strconv"
	"strings"

	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
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

// backendPolicyStatusContributions projects attachments and the base
// translation errors attributed to policies into policy status.
func backendPolicyStatusContributions(
	backends krt.Collection[*ir.BackendObjectIR],
	statusClusters krt.Collection[uccWithCluster],
	krtopts krtutil.KrtOptions,
) krt.Collection[reports.StatusContribution] {
	return krt.NewManyCollection(backends, func(kctx krt.HandlerContext, backend *ir.BackendObjectIR) []reports.StatusContribution {
		if backend == nil {
			return nil
		}
		baseKey := uccClusterResourceName(ir.UniquelyConnectedClient{}, backend.ClusterName())
		reportMap := GenerateBackendPolicyReport([]*ir.BackendObjectIR{backend}, krt.Fetch(kctx, statusClusters, krt.FilterKey(baseKey)))
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

// xdsRejectionStatusContributions turns each Gateway's outstanding xDS rejections into a
// Programmed=False overlay on its status.
func xdsRejectionStatusContributions(
	rejections krt.Collection[xds.GatewayXdsRejection],
	krtopts krtutil.KrtOptions,
) krt.Collection[reports.StatusContribution] {
	return krt.NewCollection(rejections, func(_ krt.HandlerContext, rejection xds.GatewayXdsRejection) *reports.StatusContribution {
		contribution := reports.NewXdsRejectionContribution(rejection.Gateway, xdsRejectionMessage(rejection))
		return &contribution
	}, krtopts.ToOptions("XdsRejectionStatusContributions")...)
}

// xdsRejectionMessage names each rejected resource type with Envoy's error. It contains no
// proxy or stream identity, so it changes only when the rejections themselves do.
func xdsRejectionMessage(rejection xds.GatewayXdsRejection) string {
	var b strings.Builder
	b.WriteString("Envoy rejected the latest configuration and is serving the last accepted version: ")
	for i, r := range rejection.Rejections {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(r.TypeURL)
		b.WriteString(": ")
		b.WriteString(r.Message)
		if r.Others > 0 {
			b.WriteString(" (and ")
			b.WriteString(strconv.Itoa(r.Others))
			b.WriteString(" other errors from other proxies)")
		}
	}
	return b.String()
}
