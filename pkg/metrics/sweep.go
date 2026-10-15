package metrics

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// PartialDeleter is a metric whose series can be removed by partial label match.
// Counter, Histogram, and Gauge all satisfy it.
type PartialDeleter interface {
	Metric
	DeletePartialMatch(...Label) int
}

// scopedMetric is a metric whose series each belong to one Kubernetes object, identified
// by its namespace label and, for objects other than namespaces, its name label.
type scopedMetric struct {
	m              PartialDeleter
	namespaceLabel string
	// nameLabel is empty for a metric scoped to a namespace only.
	nameLabel string
	// match restricts the sweep to series carrying these label values, for a metric that
	// also holds series for objects of another kind.
	match []Label
}

var (
	scopedLock sync.RWMutex
	scoped     []scopedMetric
)

// RegisterGatewayScoped marks m as carrying series per Gateway, identified by
// namespaceLabel and nameLabel, so a sweep removes them once the Gateway no longer
// exists. If match is set, only series carrying those label values are swept.
// It panics if m lacks any of the labels, which is a programming error caught at init.
func RegisterGatewayScoped(m PartialDeleter, namespaceLabel, nameLabel string, match ...Label) {
	register(scopedMetric{m: m, namespaceLabel: namespaceLabel, nameLabel: nameLabel, match: match})
}

// RegisterNamespaceScoped marks m as carrying series per namespace, identified by
// namespaceLabel, so a sweep removes them once the namespace no longer exists.
// Kubernetes removes a Namespace only after everything in it is gone, so this suits any
// metric whose label names the namespace of the object it describes.
// It panics if m lacks namespaceLabel.
func RegisterNamespaceScoped(m PartialDeleter, namespaceLabel string) {
	register(scopedMetric{m: m, namespaceLabel: namespaceLabel})
}

func register(s scopedMetric) {
	labels := []string{s.namespaceLabel}
	if s.nameLabel != "" {
		labels = append(labels, s.nameLabel)
	}
	for _, l := range s.match {
		labels = append(labels, l.Name)
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
}

// Run sweeps every interval until ctx is done. It skips passes until hasSynced returns
// true, since every series would look stale against caches that are still empty.
func (s Sweeper) Run(ctx context.Context, interval time.Duration, hasSynced func() bool) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if Active() && hasSynced() {
				s.Sweep()
			}
		}
	}
}

// Sweep removes the series of every registered metric whose object no longer exists,
// returning the number of series removed.
func (s Sweeper) Sweep() int {
	scopedLock.RLock()
	defer scopedLock.RUnlock()

	deleted := 0
	for _, sm := range scoped {
		for _, key := range seriesKeys(sm) {
			if s.exists(sm, key) {
				continue
			}
			labels := append([]Label{{Name: sm.namespaceLabel, Value: key.namespace}}, sm.match...)
			if sm.nameLabel != "" {
				labels = append(labels, Label{Name: sm.nameLabel, Value: key.name})
			}
			deleted += sm.m.DeletePartialMatch(labels...)
		}
	}
	return deleted
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

// seriesKeys returns the distinct objects that sm currently exports series for.
func seriesKeys(sm scopedMetric) []objectKey {
	ch := make(chan prometheus.Metric)
	go func() {
		GetPromCollector(sm.m).Collect(ch)
		close(ch)
	}()

	seen := map[objectKey]struct{}{}
	var keys []objectKey
	for m := range ch {
		var out dto.Metric
		if err := m.Write(&out); err != nil {
			continue
		}
		if !matches(out.GetLabel(), sm.match) {
			continue
		}
		key := objectKey{
			namespace: labelValue(out.GetLabel(), sm.namespaceLabel),
			name:      labelValue(out.GetLabel(), sm.nameLabel),
		}
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	return keys
}

func matches(pairs []*dto.LabelPair, match []Label) bool {
	for _, l := range match {
		if labelValue(pairs, l.Name) != l.Value {
			return false
		}
	}
	return true
}

func labelValue(pairs []*dto.LabelPair, name string) string {
	if name == "" {
		return ""
	}
	for _, p := range pairs {
		if p.GetName() == name {
			return p.GetValue()
		}
	}
	return ""
}
