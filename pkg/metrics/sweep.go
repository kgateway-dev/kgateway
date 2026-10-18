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

// scopedMetric is a metric whose series each belong to one Kubernetes object, identified
// by its namespace label and, for objects other than namespaces, its name label.
type scopedMetric struct {
	m              SeriesDeleter
	namespaceLabel string
	// nameLabel is empty for a metric scoped to a namespace only.
	nameLabel string
}

var (
	scopedLock sync.RWMutex
	scoped     []scopedMetric
)

// RegisterGatewayScoped marks m as carrying series per Gateway, identified by
// namespaceLabel and nameLabel, so a sweep removes them once the Gateway no longer
// exists. Series with an empty namespaceLabel are kept.
// It panics if m lacks either label, which is a programming error caught at init.
func RegisterGatewayScoped(m SeriesDeleter, namespaceLabel, nameLabel string) {
	register(scopedMetric{m: m, namespaceLabel: namespaceLabel, nameLabel: nameLabel})
}

// RegisterNamespaceScoped marks m as carrying series per namespace, identified by
// namespaceLabel, so a sweep removes them once the namespace no longer exists.
// Kubernetes removes a Namespace only after everything in it is gone, so this suits any
// metric whose label names the namespace of the object it describes.
// It panics if m lacks namespaceLabel.
func RegisterNamespaceScoped(m SeriesDeleter, namespaceLabel string) {
	register(scopedMetric{m: m, namespaceLabel: namespaceLabel})
}

func register(s scopedMetric) {
	labels := []string{s.namespaceLabel}
	if s.nameLabel != "" {
		labels = append(labels, s.nameLabel)
	}
	for _, l := range labels {
		if !slices.Contains(s.m.Labels(), l) {
			panic("metric does not have label " + l)
		}
	}
	if GetPromCollector(s.m) == nil {
		panic("metric is not a prometheus metric")
	}

	scopedLock.Lock()
	defer scopedLock.Unlock()

	scoped = append(scoped, s)
}

// Sweeper removes the series of registered metrics whose object no longer exists.
//
// It is level-triggered: each pass compares the exported series against the objects that
// exist now, so a series written by work still in flight when its object was deleted is
// removed by the next pass instead of being leaked until restart.
type Sweeper struct {
	// NamespaceExists reports whether the namespace exists.
	NamespaceExists func(namespace string) bool
	// GatewayExists reports whether the Gateway exists.
	GatewayExists func(namespace, name string) bool
	// HasSynced reports whether the caches behind NamespaceExists and GatewayExists have
	// synced. Run skips passes until then, since every series would look stale against
	// caches that are still empty.
	HasSynced func() bool
}

// Run sweeps every interval until ctx is done.
func (s Sweeper) Run(ctx context.Context, interval time.Duration) {
	wait.UntilWithContext(ctx, func(context.Context) {
		if Active() && s.HasSynced() {
			s.Sweep()
		}
	}, interval)
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
	ch := make(chan prometheus.Metric, 1024)
	go func() {
		GetPromCollector(sm.m).Collect(ch)
		close(ch)
	}()

	exists := map[objectKey]bool{}
	var stale [][]Label
	for m := range ch {
		var out dto.Metric
		if err := m.Write(&out); err != nil {
			continue
		}
		pairs := out.GetLabel()
		key := objectKey{
			namespace: labelValue(pairs, sm.namespaceLabel),
			name:      labelValue(pairs, sm.nameLabel),
		}
		live, ok := exists[key]
		if !ok {
			live = s.exists(sm, key)
			exists[key] = live
		}
		if !live {
			labels := make([]Label, 0, len(pairs))
			for _, p := range pairs {
				labels = append(labels, Label{Name: p.GetName(), Value: p.GetValue()})
			}
			stale = append(stale, labels)
		}
	}
	return stale
}

func (s Sweeper) exists(sm scopedMetric, key objectKey) bool {
	// Series with no namespace describe cluster-scoped objects or carry no object at all.
	if key.namespace == "" {
		return true
	}
	if sm.nameLabel == "" {
		return s.NamespaceExists(key.namespace)
	}
	return s.GatewayExists(key.namespace, key.name)
}

type objectKey struct {
	namespace string
	name      string
}

func labelValue(pairs []*dto.LabelPair, name string) string {
	for _, p := range pairs {
		if p.GetName() == name {
			return p.GetValue()
		}
	}
	return ""
}
