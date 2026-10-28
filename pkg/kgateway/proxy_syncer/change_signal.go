package proxy_syncer

import (
	"sync/atomic"

	"istio.io/istio/pkg/kube/krt"

	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// changeSignal stands in for one large row (a client's EDS or CDS payload, a
// gateway's translation output) as a dependency of the per-client snapshot
// transform. KRT queues each dependency event for a collection with the rows it
// carries, Old and New, until the collection's transform gets to it. The
// snapshot transform walks every cluster and CLA, so during a burst it falls
// behind, and each queued event would pin two full payload versions. A signal
// event pins a key and a counter; the transform reads the row itself.
type changeSignal struct {
	key string
	n   uint64
}

func (s changeSignal) ResourceName() string { return s.key }

func (s changeSignal) Equals(in changeSignal) bool {
	return s.key == in.key && s.n == in.n
}

// changeSignals emits a signal per row, changed on every row change and
// deleted with the row. Its transform only counts, so its own queue, which
// does carry full rows, drains as fast as they arrive.
func changeSignals[T any](krtopts krtutil.KrtOptions, name string, rows krt.Collection[T]) krt.Collection[changeSignal] {
	var n atomic.Uint64
	return krt.NewCollection(rows, func(_ krt.HandlerContext, row T) *changeSignal {
		return &changeSignal{key: krt.GetKey(row), n: n.Add(1)}
	}, krtopts.ToOptions(name)...)
}

// fetchSignaled fetches key's signal, registering the KRT dependency, and
// returns key's row, or nil if either is missing.
//
// The row is never older than the signal: rows stores it before delivering
// the event the signal is built from. It may be newer, which is the state KRT
// recomputes against anyway, and its own signal reruns the transform. A row
// missing behind a present signal is transient: the pending signal delete or
// update reruns the transform.
func fetchSignaled[T any](kctx krt.HandlerContext, signals krt.Collection[changeSignal], rows krt.Collection[T], key string) *T {
	if krt.FetchOne(kctx, signals, krt.FilterKey(key)) == nil {
		return nil
	}
	return rows.GetKey(key)
}
