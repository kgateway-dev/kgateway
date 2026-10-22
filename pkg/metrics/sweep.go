package metrics

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"k8s.io/apimachinery/pkg/util/wait"
)

// SeriesDeleter is a metric whose series can be removed by their full label set.
// Counter, Histogram, and Gauge all satisfy it.
type SeriesDeleter interface {
	Metric
	Delete(...Label) bool
}

// scopedMetric is a metric whose series each belong to the Gateway named by its
// namespace and name labels.
type scopedMetric struct {
	m              SeriesDeleter
	collector      prometheus.Collector
	namespaceLabel string
	nameLabel      string
}

var (
	scopedLock sync.RWMutex
	scoped     []scopedMetric
)

// RegisterGatewayScoped marks m as carrying series per Gateway, identified by
// namespaceLabel and nameLabel, so a sweep removes them once the Gateway no longer
// exists. Series with an empty namespaceLabel are kept.
// It panics if m lacks either label, which is a programming error caught at init.
//
// A sweep can remove a series while a write to it is still in flight, so a registered
// gauge must be written with Set from a value its producer owns, never with Add or Sub.
func RegisterGatewayScoped(m SeriesDeleter, namespaceLabel, nameLabel string) {
	for _, l := range []string{namespaceLabel, nameLabel} {
		if !slices.Contains(m.Labels(), l) {
			panic("metric does not have label " + l)
		}
	}
	collector := GetPromCollector(m)
	if collector == nil {
		panic("metric is not a prometheus metric")
	}

	scopedLock.Lock()
	defer scopedLock.Unlock()

	scoped = append(scoped, scopedMetric{m: m, collector: collector, namespaceLabel: namespaceLabel, nameLabel: nameLabel})
}

// Sweeper removes the series of registered metrics whose object no longer exists.
//
// It is level-triggered: each pass compares the exported series against the objects that
// exist now, so a series written by work still in flight when its object was deleted is
// removed by the next pass instead of being leaked until restart. Its caches must have
// synced before it runs, since every series would look stale against empty ones.
type Sweeper struct {
	// GatewayExists reports whether the Gateway exists.
	GatewayExists func(namespace, name string) bool
}

// Run sweeps every interval until ctx is done.
func (s Sweeper) Run(ctx context.Context, interval time.Duration) {
	wait.UntilWithContext(ctx, func(context.Context) { s.Sweep() }, interval)
}

// Sweep removes the series of every registered metric whose object no longer exists,
// returning the number of series removed.
func (s Sweeper) Sweep() int {
	scopedLock.RLock()
	defer scopedLock.RUnlock()

	deleted := 0
	for _, sm := range scoped {
		for _, labels := range s.staleSeries(sm) {
			if sm.m.Delete(labels...) {
				deleted++
			}
		}
	}
	return deleted
}

// staleSeries returns the label sets of sm's series whose object no longer exists.
func (s Sweeper) staleSeries(sm scopedMetric) [][]Label {
	// Collect holds the metric's lock while it sends, so drain it before writing each
	// series out, which is slow for histograms.
	ch := make(chan prometheus.Metric, 1024)
	go func() {
		sm.collector.Collect(ch)
		close(ch)
	}()
	var series []prometheus.Metric
	for m := range ch {
		series = append(series, m)
	}

	var stale [][]Label
	for _, m := range series {
		var out dto.Metric
		if err := m.Write(&out); err != nil {
			continue
		}
		pairs := out.GetLabel()
		if s.live(sm, pairs) {
			continue
		}
		labels := make([]Label, 0, len(pairs))
		for _, p := range pairs {
			labels = append(labels, Label{Name: p.GetName(), Value: p.GetValue()})
		}
		stale = append(stale, labels)
	}
	return stale
}

// live reports whether the Gateway of a series of sm still exists.
func (s Sweeper) live(sm scopedMetric, pairs []*dto.LabelPair) bool {
	namespace := labelValue(pairs, sm.namespaceLabel)
	// Series with no namespace describe cluster-scoped objects or carry no object at all.
	return namespace == "" || s.GatewayExists(namespace, labelValue(pairs, sm.nameLabel))
}

func labelValue(pairs []*dto.LabelPair, name string) string {
	for _, p := range pairs {
		if p.GetName() == name {
			return p.GetValue()
		}
	}
	return ""
}
