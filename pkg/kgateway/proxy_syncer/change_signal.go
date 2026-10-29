package proxy_syncer

import (
	"istio.io/istio/pkg/kube/krt"

	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// changeSignals projects payload events to keys before they enter the snapshot
// transform's queue. MapCollection has no input cache or transformation queue
// of its own, and its reads and sync state delegate to the payload collection.
//
// MapCollection forwards every source event by default, even when the mapped
// Old and New keys are equal. Fetch's dependency matching still invalidates the
// reader for those updates. Do not use WithMapDiscardEqual or PartialFetch here:
// suppressing equal-key updates would leave snapshots stale.
func changeSignals[T any](krtopts krtutil.KrtOptions, name string, rows krt.Collection[T]) krt.Collection[string] {
	return krt.MapCollection(rows, krt.GetKey[T], krtopts.ToOptions(name)...)
}

// fetchSignaled registers a dependency on key before reading its latest payload.
// signals must be the changeSignals view of rows. Fetching rows directly would
// also enqueue full payloads, defeating the key-only event projection.
//
// The subscription includes absent keys. A change between FetchOne and GetKey
// is either visible in the read or its event reruns the reader (possibly both).
func fetchSignaled[T any](kctx krt.HandlerContext, signals krt.Collection[string], rows krt.Collection[T], key string) *T {
	_ = krt.FetchOne(kctx, signals, krt.FilterKey(key))
	return rows.GetKey(key)
}
