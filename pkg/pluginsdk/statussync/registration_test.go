package statussync

import (
	"testing"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func testRegistrationInputs(stop chan struct{}) RegistrationInputs {
	contributions := krt.NewStaticCollection[reports.StatusContribution](nil, nil, krt.WithStop(stop))
	return RegistrationInputs{
		Collections:         NewStatusCollections(),
		StatusContributions: contributions,
		ContributionsByTarget: krt.NewIndex(contributions, "status-target", func(c reports.StatusContribution) []reports.StatusKey {
			return []reports.StatusKey{c.Target}
		}),
		KrtOpts: krtutil.KrtOptions{Stop: stop},
	}
}

// TestRegisterWiresAllThreeHalves checks that Register wires every half from
// RegistrationInputs alone: the raw collection sweeps on leadership, the reducer feeds the
// queue and is keyed by the GVK, and the reducer joins the sync barrier.
func TestRegisterWiresAllThreeHalves(t *testing.T) {
	stop := test.NewStop(t)
	in := testRegistrationInputs(stop)
	gvk := schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1", Kind: "HTTPRoute"}
	route := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"}}
	objects := krt.NewStaticCollection(nil, []*gwv1.HTTPRoute{route}, krt.WithStop(stop))

	reduced := Register(in, gvk, objects)
	require.True(t, reduced.WaitUntilSynced(stop))
	require.True(t, in.Collections.HasSynced(), "the reducer must be part of the sync barrier")

	want := Resource{GroupVersionKind: gvk, NamespacedName: types.NamespacedName{Namespace: "default", Name: "route"}}
	_, ok := ReportFor(reduced, want)
	require.True(t, ok, "the reduction must be keyed by the registered GVK")

	queue := &recordingQueue{}
	in.Collections.SetQueue(queue)
	// One push from the raw collection's replay, one from the reducer's.
	require.Equal(t, []Resource{want, want}, queue.awaitResources(t, 2))
}

// TestRegisterByObjectGVKKeysByTypeMeta checks that a normalized object is reduced and
// enqueued under the GVK it carries, not the fallback.
func TestRegisterByObjectGVKKeysByTypeMeta(t *testing.T) {
	stop := test.NewStop(t)
	in := testRegistrationInputs(stop)
	fallback := schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1", Kind: "ListenerSet"}
	actual := schema.GroupVersionKind{Group: "gateway.networking.x-k8s.io", Version: "v1alpha1", Kind: "XListenerSet"}
	ls := &gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: "listeners", Namespace: "default"}}
	ls.SetGroupVersionKind(actual)
	objects := krt.NewStaticCollection(nil, []*gwv1.ListenerSet{ls}, krt.WithStop(stop))

	reduced := RegisterByObjectGVK(in, fallback, objects)
	require.True(t, reduced.WaitUntilSynced(stop))

	want := Resource{GroupVersionKind: actual, NamespacedName: types.NamespacedName{Namespace: "default", Name: "listeners"}}
	_, ok := ReportFor(reduced, want)
	require.True(t, ok)

	queue := &recordingQueue{}
	in.Collections.SetQueue(queue)
	require.Equal(t, []Resource{want, want}, queue.awaitResources(t, 2))
}

func TestDesiredFromReport(t *testing.T) {
	stop := test.NewStop(t)
	in := testRegistrationInputs(stop)
	gvk := schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1", Kind: "HTTPRoute"}
	route := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"}}
	reduced := Register(in, gvk, krt.NewStaticCollection(nil, []*gwv1.HTTPRoute{route}, krt.WithStop(stop)))
	require.True(t, reduced.WaitUntilSynced(stop))

	known := Resource{GroupVersionKind: gvk, NamespacedName: types.NamespacedName{Namespace: "default", Name: "route"}}
	unknown := Resource{GroupVersionKind: gvk, NamespacedName: types.NamespacedName{Namespace: "default", Name: "gone"}}
	var built []Resource
	desired := DesiredFromReport(reduced, func(res Resource, _ reports.StatusReport, current *gwv1.HTTPRoute) *string {
		built = append(built, res)
		if current == nil {
			return nil
		}
		s := current.Name
		return &s
	})

	got, ok := desired(known, route)
	require.True(t, ok)
	require.Equal(t, "route", got)

	_, ok = desired(known, nil)
	require.False(t, ok, "a nil build suppresses the write")

	_, ok = desired(unknown, route)
	require.False(t, ok, "a resource with no reduction is not built")
	require.Equal(t, []Resource{known, known}, built)
}
