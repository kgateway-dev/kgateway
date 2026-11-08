package statussync

import (
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// RegistrationInputs exposes the keyed status pipeline to code that adds a resource type
// to it: policy plugins via PolicyPlugin.RegisterPolicyStatus, and downstream resource
// types via proxy_syncer.WithStatusRegistration. A registration builds its per-object
// report reduction and its just-in-time writer; the event handlers feeding them are
// attached only while this replica is leader.
//
// There is deliberately one struct for both entry points. They were separate types with
// identical fields, and a field added to one of them silently did not reach the other.
type RegistrationInputs struct {
	// Collections owns the raw-resource and reduced-report event sources that feed the
	// leader's status queue. Register wires both for one kind; RegisterResource and
	// RegisterResourceReports are the individual halves. Registering a reduction is also
	// what enrolls it in the StatusSyncer's cache synchronization barrier.
	Collections *StatusCollections
	// StatusContributions contains all Gateway- and Backend-produced status facts.
	StatusContributions krt.Collection[reports.StatusContribution]
	// ContributionsByTarget selects the facts belonging to one status owner.
	ContributionsByTarget krt.Index[reports.StatusKey, reports.StatusContribution]
	// KrtOpts supplies the standard collection lifecycle and debugging options.
	KrtOpts krtutil.KrtOptions
	// ControllerName is the Gateway API controller name this replica writes status as. Writers
	// use it to tell the route parents and policy ancestors they own from those of other
	// controllers sharing the object.
	ControllerName string
	// RegisterWriter registers the just-in-time writer that persists this resource's status.
	//
	// The registered writer must be idempotent: its own write echoes back as an informer
	// event that re-enqueues the resource, and nothing but the writer's live-vs-desired
	// skip stops that cycle. CheckWriterIdempotent asserts it; run it in the registration's
	// tests, as the built-in writers do.
	RegisterWriter func(gvk schema.GroupVersionKind, syncer ResourceStatusSyncer)
}

// Register wires one status kind into the pipeline described by in: it is RegisterKind with
// in's collections, contributions and index, and a reducer named after the kind. Prefer it to
// RegisterKind, whose extra arguments can only ever be these.
func Register[I controllers.Object](
	in RegistrationInputs,
	gvk schema.GroupVersionKind,
	objects krt.Collection[I],
) krt.Collection[ResourceReports] {
	return registerKind(in.Collections, objects, in.StatusContributions, in.ContributionsByTarget,
		func(I) schema.GroupVersionKind { return gvk },
		func() { RegisterResource(in.Collections, gvk, objects) },
		in.reportsOptions(gvk)...)
}

// RegisterByObjectGVK is Register for a normalized collection whose objects retain distinct
// source GVKs in TypeMeta, such as the combined ListenerSet/XListenerSet source. fallback keys
// objects without one and names the reducer.
func RegisterByObjectGVK[I controllers.Object](
	in RegistrationInputs,
	fallback schema.GroupVersionKind,
	objects krt.Collection[I],
) krt.Collection[ResourceReports] {
	return registerKind(in.Collections, objects, in.StatusContributions, in.ContributionsByTarget,
		func(obj I) schema.GroupVersionKind { return ObjectGVKOrDefault(obj, fallback) },
		func() { registerResourceByObjectGVK(in.Collections, fallback, objects) },
		in.reportsOptions(fallback)...)
}

// reportsOptions names the report reducer of gvk "<Kind>StatusReports", the name every
// registration used when it spelled it out by hand.
func (in RegistrationInputs) reportsOptions(gvk schema.GroupVersionKind) []krt.CollectionOption {
	return in.KrtOpts.ToOptions(gvk.Kind + "StatusReports")
}
