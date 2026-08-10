package statussync

import (
	"context"

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
	// Ctx is the controller's root context, captured by desired-status builders that run
	// for the lifetime of the process. It may be nil for registrations made before the
	// controller context exists; treat a nil Ctx as context.Background().
	Ctx context.Context
	// Collections owns the raw-resource and reduced-report event sources that feed the
	// leader's status queue. RegisterKind wires both for one kind; RegisterResource and
	// RegisterResourceReports are the individual halves. Registering a reduction is also
	// what enrols it in the StatusSyncer's cache synchronization barrier.
	Collections *StatusCollections
	// StatusContributions contains all Gateway- and Backend-produced status facts.
	StatusContributions krt.Collection[reports.StatusContribution]
	// ContributionsByTarget selects the facts belonging to one status owner.
	ContributionsByTarget krt.Index[reports.StatusKey, reports.StatusContribution]
	// KrtOpts supplies the standard collection lifecycle and debugging options.
	KrtOpts krtutil.KrtOptions
	// RegisterWriter registers the just-in-time writer that persists this resource's status.
	RegisterWriter func(gvk schema.GroupVersionKind, syncer ResourceStatusSyncer)
}
