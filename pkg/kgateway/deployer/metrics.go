package deployer

import (
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrappatch"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	metricsSubsystem   = "controller"
	namespaceLabel     = "namespace"
	nameLabel          = "name"
	managedPathLabel   = "path"
	managedPathsMetric = "bootstrap_patch_managed_paths"
)

// bootstrapPatchManagedPaths has a series, set to 1, for each managed path the
// bootstrap patches of a Gateway set. A Gateway whose patches set no managed path has
// no series.
var bootstrapPatchManagedPaths = metrics.NewGauge(
	metrics.GaugeOpts{
		Subsystem: metricsSubsystem,
		Name:      managedPathsMetric,
		Help:      "Control-plane-managed bootstrap paths that a Gateway's GatewayParameters bootstrap patches set. The value is always 1.",
	},
	[]string{namespaceLabel, nameLabel, managedPathLabel},
)

// recordBootstrapPatchManagedPaths replaces the Gateway's series with one for each
// path in paths.
func recordBootstrapPatchManagedPaths(gw types.NamespacedName, paths []bootstrappatch.ManagedPath) {
	if !metrics.Active() {
		return
	}

	// Drop the series for paths the patches no longer set.
	ForgetBootstrapPatchMetrics(gw)

	// Add one series per path; two GatewayParameters that set the same path share it.
	for _, p := range paths {
		bootstrapPatchManagedPaths.Set(1,
			metrics.Label{Name: namespaceLabel, Value: gw.Namespace},
			metrics.Label{Name: nameLabel, Value: gw.Name},
			metrics.Label{Name: managedPathLabel, Value: p.Path},
		)
	}
}

// ForgetBootstrapPatchMetrics removes the bootstrap patch series of a Gateway. Call it
// when the Gateway is deleted.
func ForgetBootstrapPatchMetrics(gw types.NamespacedName) {
	if !metrics.Active() {
		return
	}
	bootstrapPatchManagedPaths.DeletePartialMatch(
		metrics.Label{Name: namespaceLabel, Value: gw.Namespace},
		metrics.Label{Name: nameLabel, Value: gw.Name},
	)
}

// ResetMetrics resets the metrics from this package.
// This is provided for testing purposes only.
func ResetMetrics() {
	bootstrapPatchManagedPaths.Reset()
}
