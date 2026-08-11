package fake

import (
	"reflect"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

// ReactorClientset is the part of a generated fake clientset needed to correct its status
// subresource behavior.
type ReactorClientset interface {
	PrependReactor(verb, resource string, reaction k8stesting.ReactionFunc)
	Tracker() k8stesting.ObjectTracker
}

// InstallStatusSubresourceReactor makes UpdateStatus behave the way the API server does:
// only status is taken from the request and the rest of the stored object is left alone.
//
// client-go's fake tracker documents that "subresources are not handled accurately" and
// replaces the entire stored object on a status update. Every kgateway status writer sends
// an ObjectMeta carrying just name, namespace and resourceVersion — the API server ignores
// everything else on a status write — so against an uncorrected fake the first status write
// erases the object's spec. Anything that then rebuilds status from spec (which is every
// status builder we have) sees a different object than it did a moment ago and writes again,
// so a test of write convergence measures the fake's infidelity rather than the controller.
//
// It corrects that one behavior only. In particular the tracker still does no
// resourceVersion checking, so this does not give status writes optimistic concurrency.
func InstallStatusSubresourceReactor(clientsets ...ReactorClientset) {
	for _, cs := range clientsets {
		tracker := cs.Tracker()
		cs.PrependReactor("update", "*", statusSubresourceReaction(tracker))
	}
}

func statusSubresourceReaction(tracker k8stesting.ObjectTracker) k8stesting.ReactionFunc {
	return func(action k8stesting.Action) (bool, runtime.Object, error) {
		update, ok := action.(k8stesting.UpdateActionImpl)
		if !ok || update.GetSubresource() != "status" {
			return false, nil, nil
		}
		accessor, err := meta.Accessor(update.GetObject())
		if err != nil {
			return true, nil, err
		}
		gvr, ns := update.GetResource(), update.GetNamespace()
		existing, err := tracker.Get(gvr, ns, accessor.GetName(), metav1.GetOptions{})
		if err != nil {
			// Let the default reaction produce the NotFound (or whatever else) the caller
			// would have seen.
			return false, nil, nil
		}
		merged := existing.DeepCopyObject()
		if !copyStatus(update.GetObject(), merged) {
			// Not a typed object with a Status field (an unstructured update, say). Leave it
			// to the default reaction rather than guessing.
			return false, nil, nil
		}
		if err := tracker.Update(gvr, merged, ns); err != nil {
			return true, nil, err
		}
		return true, merged, nil
	}
}

// copyStatus overwrites dst's Status field with src's, reporting whether it could. Both are
// the same Go type whenever the update came from a typed client.
func copyStatus(src, dst runtime.Object) bool {
	srcValue, dstValue := reflect.ValueOf(src), reflect.ValueOf(dst)
	if srcValue.Kind() != reflect.Pointer || dstValue.Kind() != reflect.Pointer ||
		srcValue.Elem().Kind() != reflect.Struct || dstValue.Elem().Kind() != reflect.Struct {
		return false
	}
	srcStatus := srcValue.Elem().FieldByName("Status")
	dstStatus := dstValue.Elem().FieldByName("Status")
	if !srcStatus.IsValid() || !dstStatus.IsValid() || !dstStatus.CanSet() ||
		srcStatus.Type() != dstStatus.Type() {
		return false
	}
	dstStatus.Set(srcStatus)
	return true
}
