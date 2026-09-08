// ON_EXPERIMENTAL_PROMOTION : Delete this file with xlistenerset_status.go.
// Ref: https://github.com/kgateway-dev/kgateway/issues/12827

package proxy_syncer

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

const legacyNamespace = "default"

// legacyListenerSet is a normalized legacy XListenerSet as the converting collection hands it
// to the writer: a *gwv1.ListenerSet that kept its legacy GVK in TypeMeta, which is what keys
// both its report and its writer. One listener carries an explicit port and one relies on its
// protocol's default, so the injected status ports cover both.
func legacyListenerSet() *gwv1.ListenerSet {
	ls := &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "legacy-ls", Namespace: legacyNamespace, Generation: 3, ResourceVersion: "7",
		},
		Spec: gwv1.ListenerSetSpec{
			ParentRef: gwv1.ParentGatewayReference{Name: "gw"},
			Listeners: []gwv1.ListenerEntry{
				{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 8080},
				{Name: "https", Protocol: gwv1.HTTPSProtocolType},
			},
		},
		Status: gwv1.ListenerSetStatus{
			Conditions: []metav1.Condition{{
				Type:               string(gwv1.ListenerSetConditionAccepted),
				Status:             metav1.ConditionFalse,
				Reason:             string(gwv1.ListenerSetReasonPending),
				ObservedGeneration: 2,
				LastTransitionTime: staleTime(),
			}},
		},
	}
	ls.SetGroupVersionKind(wellknown.XListenerSetGVK)
	return ls
}

type xListenerSetFixture struct {
	syncer  *xListenerSetStatusSyncer
	objects krt.StaticCollection[*gwv1.ListenerSet]
	dynamic *dynamicfake.FakeDynamicClient
	res     statussync.Resource
}

// newXListenerSetFixture wires the legacy writer over static collections. The dynamic client
// is the only part of the pipeline the legacy flavor does not share with the promoted one, so
// it is the only part that needs a fake API server here.
func newXListenerSetFixture(t *testing.T, ls *gwv1.ListenerSet) xListenerSetFixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	// The report comes from the real reporter and is keyed by the object's own GVK, so a
	// writer reading reports under the promoted GVK would find nothing here.
	reportMap := reports.NewReportMap()
	reports.NewReporter(&reportMap).ListenerSet(ls)
	nn := types.NamespacedName{Namespace: ls.Namespace, Name: ls.Name}
	reportCol := krt.NewStaticCollection(nil, []statussync.ResourceReports{{
		Resource: statussync.Resource{GroupVersionKind: wellknown.XListenerSetGVK, NamespacedName: nn},
		Report:   reports.StatusReport{ListenerSet: reportMap.ListenerSet(ls)},
	}}, krtopts.ToOptions("XListenerSetStatusReports")...)
	objects := krt.NewStaticCollection(nil, []*gwv1.ListenerSet{ls},
		krtopts.ToOptions("XListenerSets")...)

	c := apifake.NewClient(t)
	return xListenerSetFixture{
		syncer: &xListenerSetStatusSyncer{
			col:     objects,
			client:  c,
			reports: reportCol,
		},
		objects: objects,
		dynamic: c.Dynamic().(*dynamicfake.FakeDynamicClient),
		res: statussync.Resource{
			GroupVersionKind: wellknown.XListenerSetGVK,
			NamespacedName:   nn,
		},
	}
}

func (f xListenerSetFixture) apply() {
	f.syncer.ApplyStatus(context.Background(), f.res)
}

// statusPatches returns the bodies of the status merge patches sent to the legacy GVR.
func (f xListenerSetFixture) statusPatches() [][]byte {
	var patches [][]byte
	for _, a := range f.dynamic.Actions() {
		patch, ok := a.(k8stesting.PatchAction)
		if !ok || a.GetSubresource() != "status" || a.GetResource() != wellknown.XListenerSetGVR {
			continue
		}
		patches = append(patches, patch.GetPatch())
	}
	return patches
}

// TestXListenerSetStatusPatchPayload pins the one thing the legacy flavor does differently
// from the promoted one: the status goes out as a merge patch against the legacy GVR, carrying
// the resourceVersion the status was built from and the per-listener port the legacy CRD
// schema requires but gwv1.ListenerSetStatus cannot hold.
func TestXListenerSetStatusPatchPayload(t *testing.T) {
	f := newXListenerSetFixture(t, legacyListenerSet())

	f.apply()

	patches := f.statusPatches()
	require.Len(t, patches, 1, "the stale status must be corrected in one patch")

	var body struct {
		Metadata struct {
			ResourceVersion string `json:"resourceVersion"`
		} `json:"metadata"`
		Status struct {
			Conditions []metav1.Condition `json:"conditions"`
			Listeners  []struct {
				Name string `json:"name"`
				Port int64  `json:"port"`
			} `json:"listeners"`
		} `json:"status"`
	}
	require.NoError(t, json.Unmarshal(patches[0], &body))

	require.Equal(t, "7", body.Metadata.ResourceVersion,
		"the patch must carry the resourceVersion it was built from, or a merge patch applies unconditionally")
	accepted := meta.FindStatusCondition(body.Status.Conditions, string(gwv1.ListenerSetConditionAccepted))
	require.NotNil(t, accepted, "our Accepted condition must be published")
	require.Equal(t, metav1.ConditionTrue, accepted.Status)
	require.Equal(t, int64(3), accepted.ObservedGeneration)

	ports := map[string]int64{}
	for _, l := range body.Status.Listeners {
		ports[l.Name] = l.Port
	}
	require.Equal(t, map[string]int64{"http": 8080, "https": 443}, ports,
		"every status listener needs the port the legacy schema requires, defaulted from its protocol")
}

// TestXListenerSetWriterIsIdempotent gives the legacy writer the convergence guarantee the
// rest of the pipeline has. It is a standard statussync.Writer, so the shared harness applies
// to it unchanged.
func TestXListenerSetWriterIsIdempotent(t *testing.T) {
	ls := legacyListenerSet()
	f := newXListenerSetFixture(t, ls)
	w := f.syncer.writer(context.Background())

	require.True(t, statussync.WriterWouldWrite(w, ls),
		"the seeded legacy status must actually be written, or the check below proves nothing")
	require.NoError(t, statussync.CheckWriterIdempotent(w, ls,
		func(current *gwv1.ListenerSet, status gwv1.ListenerSetStatus) *gwv1.ListenerSet {
			next := current.DeepCopy()
			next.Status = *status.DeepCopy()
			return next
		}))
}

// TestXListenerSetStatusSkipsNoOpPatch proves the shared live-vs-desired skip reaches the
// legacy path, which used to implement it itself: the writer is asked again about the status
// it just published -- as the informer echo does in production -- and patches nothing.
func TestXListenerSetStatusSkipsNoOpPatch(t *testing.T) {
	ls := legacyListenerSet()
	f := newXListenerSetFixture(t, ls)

	f.apply()
	require.Len(t, f.statusPatches(), 1, "the stale status must be corrected in one patch")

	// Echo our own write back into the collection the writer reads, as the informer would.
	published, ok := f.syncer.writer(context.Background()).Desired(ls)
	require.True(t, ok, "the writer must have a status for the seeded listener set")
	echoed := ls.DeepCopy()
	echoed.Status = published
	f.objects.UpdateObject(echoed)

	f.apply()
	f.apply()
	require.Len(t, f.statusPatches(), 1,
		"rebuilding from the status we just wrote must produce the same status, or patches never stop")
}
