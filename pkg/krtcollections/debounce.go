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
	out := krt.NewMutableCollection[T](seededSyncer{src: src, seeded: seeded}, nil, opts...)

	// Every changed key reaches the debouncer, which merges them into one set per flush. Until the debouncer
	// starts, keys wait in the source's handler queue.
	changed := make(chan string)
	reg := src.RegisterBatch(func(events []krt.Event[T]) {
		for _, ev := range events {
			select {
			case changed <- krt.GetKey(ev.Latest()):
			case <-stop:
				return
			}
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

		var debouncer concurrent.Debouncer[string]
		debouncer.Run(changed, stop, window, maxDelay, func(keys sets.Set[string]) {
			if keys.Len() == 1 {
				// A lone change, the common case outside bursts: apply just that key rather than comparing
				// every object.
				for k := range keys {
					if obj := src.GetKey(k); obj != nil {
						out.ConditionalUpdateObject(*obj)
					} else {
						out.DeleteObject(k)
					}
				}
				return
			}
			// Several changes must reach dependents as one event set, and Reset is the only way to publish one.
			// Applying them key by key would deliver one event set, and so one rebuild, per change.
			out.Reset(src.List())
		})
	}()
	return out.AsCollection()
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
