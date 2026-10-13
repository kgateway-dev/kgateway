package krtcollections

import (
	"time"

	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/util/concurrent"
	"istio.io/istio/pkg/util/sets"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// newDebouncedCollection mirrors src and delivers its changes in batches: a burst of changes reaches dependents as
// a single event set, flushed once no change has arrived for window, and at least once per maxDelay while changes
// keep arriving.
//
// A krt collection queues one recomputation per event set it receives, and each recomputation reruns the
// transformation against the latest state. A Gateway's translation reads all of its routes, so without batching
// a burst of N route changes rebuilds every affected Gateway N times, each a full translation. Batching the burst
// gives one rebuild per flush.
//
// A non-positive window disables batching and returns src unchanged.
func newDebouncedCollection[T any](
	src krt.Collection[T],
	window, maxDelay time.Duration,
	stop <-chan struct{},
	opts ...krt.CollectionOption,
) krt.Collection[T] {
	if window <= 0 {
		return src
	}
	maxDelay = max(maxDelay, window)

	seeded := make(chan struct{})
	out := krt.NewStaticCollection[T](seededSyncer{src: src, seeded: seeded}, nil, opts...)

	dirty := make(chan struct{}, 1)
	reg := src.RegisterBatch(func([]krt.Event[T]) {
		select {
		case dirty <- struct{}{}:
		default:
			// A flush is already pending and will read the latest state.
		}
	}, false)

	go func() {
		defer reg.UnregisterHandler()
		if !src.WaitUntilSynced(stop) {
			return
		}
		// Seed before reporting synced, so dependents never observe a partial mirror.
		out.Reset(src.List())
		close(seeded)

		var debouncer concurrent.Debouncer[struct{}]
		debouncer.Run(dirty, stop, window, maxDelay, func(sets.Set[struct{}]) {
			// Reset distributes every difference from the previous flush as one event set.
			out.Reset(src.List())
		})
	}()
	return out
}

// debounceForTranslation batches changes to a translation input as configured by the translation debounce settings.
func debounceForTranslation[T any](
	c krt.Collection[T],
	krtopts krtutil.KrtOptions,
	settings apisettings.Settings,
	name string,
) krt.Collection[T] {
	return newDebouncedCollection(c, settings.TranslationDebounce, settings.TranslationDebounceMax, krtopts.Stop, krtopts.ToOptions(name)...)
}

// seededSyncer reports synced once src has synced and the mirror has been seeded from it.
type seededSyncer struct {
	src    krt.Syncer
	seeded <-chan struct{}
}

func (s seededSyncer) HasSynced() bool {
	select {
	case <-s.seeded:
		return s.src.HasSynced()
	default:
		return false
	}
}

func (s seededSyncer) WaitUntilSynced(stop <-chan struct{}) bool {
	if !s.src.WaitUntilSynced(stop) {
		return false
	}
	select {
	case <-s.seeded:
		return true
	case <-stop:
		return false
	}
}
