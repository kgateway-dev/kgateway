package deployer

import (
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrapoverlay"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	metricsSubsystem   = "controller"
	namespaceLabel     = "namespace"
	nameLabel          = "name"
	managedPathLabel   = "path"
	managedPathsMetric = "bootstrap_overlay_managed_paths"
)

// bootstrapOverlayManagedPaths has a series, set to 1, for each managed path the
// bootstrap overlays of a Gateway set. A Gateway whose overlays set no managed path has no series.
var bootstrapOverlayManagedPaths = metrics.NewGauge(
	metrics.GaugeOpts{
		Subsystem: metricsSubsystem,
		Name:      managedPathsMetric,
		Help:      "Control-plane-managed bootstrap paths that a Gateway's GatewayParameters bootstrap overlay sets. The value is always 1.",
	},
	[]string{namespaceLabel, nameLabel, managedPathLabel},
)

// recordBootstrapOverlayManagedPaths replaces the Gateway's series with one for each
// path in paths.
func recordBootstrapOverlayManagedPaths(gw types.NamespacedName, paths []bootstrapoverlay.ManagedPath) {
	if !metrics.Active() {
		return
	}

	// Drop the series for paths the overlays no longer set.
	ForgetBootstrapOverlayMetrics(gw)

	// Add one series per path; two GatewayParameters that set the same path share it.
	for _, p := range paths {
		bootstrapOverlayManagedPaths.Set(1,
			metrics.Label{Name: namespaceLabel, Value: gw.Namespace},
			metrics.Label{Name: nameLabel, Value: gw.Name},
			metrics.Label{Name: managedPathLabel, Value: p.Path},
		)
	}
}

// ForgetBootstrapOverlayMetrics removes the bootstrap overlay series of a Gateway. Call it
// when the Gateway is deleted.
func ForgetBootstrapOverlayMetrics(gw types.NamespacedName) {
	if !metrics.Active() {
		return
	}
	bootstrapOverlayManagedPaths.DeletePartialMatch(
		metrics.Label{Name: namespaceLabel, Value: gw.Namespace},
		metrics.Label{Name: nameLabel, Value: gw.Name},
	)
}

// ResetMetrics resets the metrics from this package.
// This is provided for testing purposes only.
func ResetMetrics() {
	bootstrapOverlayManagedPaths.Reset()
}
