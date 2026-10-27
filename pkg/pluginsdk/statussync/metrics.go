package statussync

import (
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	statusSubsystem = "status"
	kindLabel       = "kind"
	resultLabel     = "result"
)

// writeResult is the outcome of one ApplyStatus call for a resource the writer has a status for.
type writeResult string

const (
	// writeResultWritten means the status differed from the live one and was persisted.
	writeResultWritten writeResult = "written"
	// writeResultUnchanged means the live status already matched, so nothing was written.
	writeResultUnchanged writeResult = "unchanged"
	// writeResultConflict means the API server held a newer resourceVersion. The informer
	// delivers it and re-enqueues the resource, so this is expected and self-heals.
	writeResultConflict writeResult = "conflict"
	// writeResultNotFound means the resource was deleted between the read and the write.
	writeResultNotFound writeResult = "not_found"
	// writeResultError means the write still failed after every retry. Nothing re-enqueues
	// the resource, so its status stays stale until an unrelated event changes it.
	writeResultError writeResult = "error"
)

var statusWritesTotal = metrics.NewCounter(
	metrics.CounterOpts{
		Subsystem: statusSubsystem,
		Name:      "writes_total",
		Help:      "Total number of status write attempts by resource kind and result. result=\"error\" counts writes that failed after every retry.",
	},
	[]string{kindLabel, resultLabel},
)

func recordStatusWrite(kind string, result writeResult) {
	if !metrics.Active() {
		return
	}
	statusWritesTotal.Inc(
		metrics.Label{Name: kindLabel, Value: kind},
		metrics.Label{Name: resultLabel, Value: string(result)},
	)
}

// ResetMetrics resets the metrics from this package.
// This is provided for testing purposes only.
func ResetMetrics() {
	statusWritesTotal.Reset()
}
