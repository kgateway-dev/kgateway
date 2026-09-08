package statussync

import (
	"encoding/json"
	"fmt"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
)

// ApplyStatusFn returns current as the API server would store it after the writer published
// status: the same object with its status replaced. It must not mutate current.
type ApplyStatusFn[O controllers.ComparableObject, S any] func(current O, status S) O

// WriterWouldWrite reports whether the writer would issue an API write for current, without
// issuing one. CheckWriterIdempotent passes trivially for an object the writer declines to
// write, so a test that means to exercise convergence should assert this first rather than
// silently checking nothing.
func WriterWouldWrite[O controllers.ComparableObject, S any](w Writer[O, S], current O) bool {
	if controllers.IsNil(current) {
		return false
	}
	decision := w.decide(current)
	return decision.has && decision.write
}

// CheckWriterIdempotent asserts the fixed-point invariant the whole status pipeline rests
// on, for one writer and one object.
//
// Every status write we make echoes back as an informer event on the collection that
// enqueues the resource, so the writer is always asked a second time about the status it
// just wrote. The only thing that terminates that cycle is the writer's live-vs-desired
// skip, which holds exactly when rebuilding from what we wrote reproduces what we wrote:
//
//	Merge(current, Desired(current)) applied to current == Merge(next, Desired(next))
//
// A builder that stamps a fresh LastTransitionTime on an unchanged condition, or that
// renormalizes (sorts, defaults, re-cases) entries it copied from the live object, breaks
// that equality and converts the designed one-shot echo into a permanent write loop against
// the API server — one that no test of the skip mechanism alone will catch, because the skip
// is working correctly and simply never fires.
//
// It returns nil when the writer would not write at all (no desired status, or already up to
// date): nothing was published, so there is nothing to converge from.
//
// Every writer registered with the status pipeline should run this, including downstream
// writers added via WithStatusRegistration — the invariant is a property of the pipeline,
// not of the writers that shipped with it.
func CheckWriterIdempotent[O controllers.ComparableObject, S any](
	w Writer[O, S],
	current O,
	apply ApplyStatusFn[O, S],
) error {
	if controllers.IsNil(current) {
		return fmt.Errorf("status writer %q: current object is nil, nothing to check", w.Name)
	}
	first := w.decide(current)
	if !first.has || !first.write {
		return nil
	}

	next := apply(current, first.status)
	if controllers.IsNil(next) {
		return fmt.Errorf("status writer %q: apply returned a nil object", w.Name)
	}
	// A harness whose apply does not actually store the published status would report the
	// writer as non-idempotent for a reason that has nothing to do with the writer, so
	// separate the two failures.
	if w.GetStatus != nil && !krt.Equal(w.GetStatus(next), first.status) {
		return fmt.Errorf("status writer %q: apply did not store the published status: stored %s, published %s",
			w.Name, renderStatus(w.GetStatus(next)), renderStatus(first.status))
	}

	second := w.decide(next)
	if !second.has || !second.write {
		return nil
	}
	published, wanted := renderStatus(first.status), renderStatus(second.status)
	hint := ""
	if published == wanted {
		// metav1.Time serializes to whole seconds, so the most common cause of this failure
		// is also the one the rendering hides.
		hint = " (the two render identically, so the difference is below the rendered " +
			"precision -- almost always a regenerated sub-second LastTransitionTime)"
	}
	return fmt.Errorf(
		"status writer %q is not idempotent: after applying the status it published, it wants to write again, "+
			"which is an unbounded write loop against the API server (each write re-enqueues the resource)%s. "+
			"published %s, then wanted %s",
		w.Name, hint, published, wanted)
}

// renderStatus formats a status for a failure message. Status types are API types, so JSON
// is both faithful and readable; %+v would print pointer addresses for the many optional
// fields Gateway API statuses carry.
func renderStatus(status any) string {
	encoded, err := json.Marshal(status)
	if err != nil {
		return fmt.Sprintf("%+v", status)
	}
	return string(encoded)
}
