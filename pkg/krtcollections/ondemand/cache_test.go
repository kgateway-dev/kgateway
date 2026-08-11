package ondemand

import (
	"context"
	"testing"
	"time"

	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func secret(ns, name string, labels map[string]string, data string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Data:       map[string][]byte{"key": []byte(data)},
	}
}

// setup builds a Cache over the fake client. refs is the initial reference set,
// already populated before SetRefs, which is how production wires it: the ref
// collections are informer-derived and synced before the cache starts. The
// returned collection stays mutable so tests can add and withdraw references.
func setup(t *testing.T, refs []ResourceRef, objs ...*corev1.Secret) (*Cache[*corev1.Secret], krt.StaticCollection[ResourceRef], *fakeWriter) {
	t.Helper()

	cobjs := make([]client.Object, 0, len(objs))
	for _, o := range objs {
		cobjs = append(cobjs, o)
	}
	c := fake.NewClient(t, cobjs...)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })

	krtOpts := krtutil.NewKrtOptions(stop, nil)
	cache := New(ctx, c, krtOpts, Config[*corev1.Secret]{
		Name: "Secrets",
		Kind: "Secret",
		GVR:  gvr.Secret,
		Getter: func(ctx context.Context, ns, name string) (*corev1.Secret, error) {
			return c.Kube().CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
		},
	})

	refCol := krt.NewStaticCollection(nil, refs, krtOpts.ToOptions("refs")...)
	cache.SetRefs(ctx, refCol)
	c.RunAndWait(stop)

	return cache, refCol, &fakeWriter{t: t, c: c}
}

func TestCacheFetchesOnlyReferencedObjects(t *testing.T) {
	referenced := secret("ns", "referenced", nil, "v1")
	ignored := secret("ns", "ignored", nil, "v1")

	cache, _, _ := setup(t, []ResourceRef{NewRef("policy", "Secret", "ns", "referenced")}, referenced, ignored)

	waitFor(t, cache.HasSynced, "cache should sync once the referenced set is fetched")

	if got := cache.Collection().GetKey("ns/referenced"); got == nil {
		t.Fatal("referenced Secret should be in the collection")
	} else if string((*got).Data["key"]) != "v1" {
		t.Fatalf("referenced Secret should carry its contents, got %q", (*got).Data["key"])
	}

	// The unreferenced Secret exists in the cluster but must never be loaded --
	// that is the entire point of the design.
	if cache.Collection().GetKey("ns/ignored") != nil {
		t.Fatal("unreferenced Secret must not be fetched")
	}
	if !cache.Exists("ns", "ignored") {
		t.Fatal("the metadata watch should still know the unreferenced Secret exists")
	}
}

func TestCacheTracksUpdatesToReferencedObjects(t *testing.T) {
	s := secret("ns", "s", nil, "before")
	cache, _, w := setup(t, []ResourceRef{NewRef("policy", "Secret", "ns", "s")}, s)
	waitFor(t, cache.HasSynced, "cache should sync")

	w.update(secret("ns", "s", nil, "after"))

	waitFor(t, func() bool {
		got := cache.Collection().GetKey("ns/s")
		return got != nil && string((*got).Data["key"]) == "after"
	}, "an update to a referenced Secret should be picked up via the metadata watch")
}

func TestCacheDropsObjectWhenReferenceIsWithdrawn(t *testing.T) {
	s := secret("ns", "s", nil, "v1")
	ref := NewRef("policy", "Secret", "ns", "s")
	cache, refs, _ := setup(t, []ResourceRef{ref}, s)
	waitFor(t, cache.HasSynced, "cache should sync")
	waitFor(t, func() bool { return cache.Collection().GetKey("ns/s") != nil }, "Secret should be loaded")

	// Deleting the policy that referenced it must evict the contents, otherwise
	// the cache would grow without bound as configuration churns.
	refs.DeleteObject(ref.ResourceName())

	waitFor(t, func() bool {
		return cache.Collection().GetKey("ns/s") == nil
	}, "withdrawing the last reference should evict the Secret")
}

func TestCacheDropsDeletedObject(t *testing.T) {
	s := secret("ns", "s", nil, "v1")
	cache, _, w := setup(t, []ResourceRef{NewRef("policy", "Secret", "ns", "s")}, s)
	waitFor(t, cache.HasSynced, "cache should sync")
	waitFor(t, func() bool { return cache.Collection().GetKey("ns/s") != nil }, "Secret should be loaded")

	w.delete("ns", "s")

	waitFor(t, func() bool {
		return cache.Collection().GetKey("ns/s") == nil
	}, "deleting a referenced Secret should remove it from the collection")
}

func TestCacheResolvesSelectorRefsAgainstMetadata(t *testing.T) {
	matching := secret("ns", "matching", map[string]string{"app": "api"}, "v1")
	other := secret("ns", "other", map[string]string{"app": "web"}, "v1")

	cache, _, w := setup(t,
		[]ResourceRef{NewSelectorRef("policy", "Secret", "", map[string]string{"app": "api"})},
		matching, other)
	waitFor(t, cache.HasSynced, "cache should sync")

	if cache.Collection().GetKey("ns/matching") == nil {
		t.Fatal("Secret matching the selector should be fetched")
	}
	if cache.Collection().GetKey("ns/other") != nil {
		t.Fatal("Secret not matching the selector must not be fetched")
	}

	// A Secret that gains a matching label later must be picked up: labels live on
	// PartialObjectMetadata, so the metadata watch is enough to notice.
	w.create(secret("ns", "late", map[string]string{"app": "api"}, "v1"))
	waitFor(t, func() bool {
		return cache.Collection().GetKey("ns/late") != nil
	}, "a newly created Secret matching the selector should be fetched")
}

func TestCacheHandlesMissingObject(t *testing.T) {
	cache, _, w := setup(t, []ResourceRef{NewRef("policy", "Secret", "ns", "later")})

	// A reference to something that does not exist must not block startup; absence
	// from the collection is how translation reports "not found".
	waitFor(t, cache.HasSynced, "cache should sync even when a reference dangles")
	if cache.Collection().GetKey("ns/later") != nil {
		t.Fatal("a dangling reference must not produce an entry")
	}

	// When the object shows up, it should be loaded without any further prompting.
	w.create(secret("ns", "later", nil, "v1"))
	waitFor(t, func() bool {
		return cache.Collection().GetKey("ns/later") != nil
	}, "creating a referenced Secret should load it")
}

func TestRefResourceNameDistinguishesSources(t *testing.T) {
	a := NewRef("policyA", "Secret", "ns", "s")
	b := NewRef("policyB", "Secret", "ns", "s")
	if a.ResourceName() == b.ResourceName() {
		t.Fatal("two policies referencing the same Secret must produce distinct keys, " +
			"otherwise KRT collapses them and withdrawing one drops the other")
	}

	sel1 := NewSelectorRef("p", "Secret", "ns", map[string]string{"a": "1"})
	sel2 := NewSelectorRef("p", "Secret", "ns", map[string]string{"a": "2"})
	if sel1.ResourceName() == sel2.ResourceName() {
		t.Fatal("distinct selectors on one policy must produce distinct keys")
	}
}

func TestRefEqualsComparesAllFields(t *testing.T) {
	base := ResourceRef{Kind: "Secret", Namespace: "ns", Name: "s", Source: "p"}
	cases := map[string]ResourceRef{
		"kind":      {Kind: "ConfigMap", Namespace: "ns", Name: "s", Source: "p"},
		"namespace": {Kind: "Secret", Namespace: "other", Name: "s", Source: "p"},
		"name":      {Kind: "Secret", Namespace: "ns", Name: "t", Source: "p"},
		"source":    {Kind: "Secret", Namespace: "ns", Name: "s", Source: "q"},
		"labels":    {Kind: "Secret", Namespace: "ns", Name: "s", Source: "p", MatchLabels: map[string]string{"a": "1"}},
	}
	if !base.Equals(base) {
		t.Fatal("a ref should equal itself")
	}
	for field, other := range cases {
		if base.Equals(other) {
			t.Fatalf("Equals ignores %s; KRT would miss the change", field)
		}
	}
}

func TestStripMetadataDropsBulkFields(t *testing.T) {
	md := &metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "s",
			Namespace: "ns",
			Labels:    map[string]string{"app": "api"},
			// kubectl stores a full copy of the object here, which would defeat
			// the point of watching metadata only.
			Annotations:   map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{...}"},
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubectl"}},
		},
	}
	out, err := stripMetadata(md)
	if err != nil {
		t.Fatalf("stripMetadata: %v", err)
	}
	got := out.(*metav1.PartialObjectMetadata)
	if got.Annotations != nil {
		t.Error("annotations should be dropped; they can hold a full copy of the object")
	}
	if got.ManagedFields != nil {
		t.Error("managed fields should be dropped")
	}
	if got.Labels["app"] != "api" {
		t.Error("labels must be kept; selector refs are resolved against them")
	}
}

// fakeWriter mutates Secrets through the typed client so the metadata watch
// observes the change the same way it would against a real API server.
type fakeWriter struct {
	t *testing.T
	c kube.Client
}

func (w *fakeWriter) create(s *corev1.Secret) {
	w.t.Helper()
	if _, err := w.c.Kube().CoreV1().Secrets(s.Namespace).Create(context.Background(), s, metav1.CreateOptions{}); err != nil {
		w.t.Fatalf("create secret: %v", err)
	}
}

func (w *fakeWriter) update(s *corev1.Secret) {
	w.t.Helper()
	if _, err := w.c.Kube().CoreV1().Secrets(s.Namespace).Update(context.Background(), s, metav1.UpdateOptions{}); err != nil {
		w.t.Fatalf("update secret: %v", err)
	}
}

func (w *fakeWriter) delete(ns, name string) {
	w.t.Helper()
	if err := w.c.Kube().CoreV1().Secrets(ns).Delete(context.Background(), name, metav1.DeleteOptions{}); err != nil {
		w.t.Fatalf("delete secret: %v", err)
	}
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}

func TestDedupeCollapsesRepeatedRefs(t *testing.T) {
	// Two HTTPS listeners sharing one certificate produce the same ref twice.
	// KRT rejects duplicate keys from a single transformation, so producers must
	// collapse them.
	src := "Gateway/ns/gw"
	refs := []ResourceRef{
		NewRef(src, "Secret", "ns", "cert"),
		NewRef(src, "Secret", "ns", "cert"),
		NewRef(src, "Secret", "ns", "other"),
	}
	got := Dedupe(refs)
	if len(got) != 2 {
		t.Fatalf("expected 2 refs after dedupe, got %d: %v", len(got), got)
	}
	seen := map[string]bool{}
	for _, r := range got {
		if seen[r.ResourceName()] {
			t.Fatalf("duplicate key survived dedupe: %s", r.ResourceName())
		}
		seen[r.ResourceName()] = true
	}

	// Refs from different owners are not duplicates and must both survive.
	both := Dedupe([]ResourceRef{
		NewRef("policyA", "Secret", "ns", "cert"),
		NewRef("policyB", "Secret", "ns", "cert"),
	})
	if len(both) != 2 {
		t.Fatalf("refs from distinct owners must not be collapsed, got %d", len(both))
	}
}

// TestSelectorRetractsWhenLabelsStopMatching is the shrinking half of selector
// support. A Secret that stops matching must have its payload evicted; if only
// the growing direction worked, a cluster where labels churn would accumulate
// payloads for Secrets nothing selects any more.
func TestSelectorRetractsWhenLabelsStopMatching(t *testing.T) {
	matching := secret("ns", "matching", map[string]string{"app": "api"}, "v1")

	cache, _, w := setup(t,
		[]ResourceRef{NewSelectorRef("policy", "Secret", "", map[string]string{"app": "api"})},
		matching)
	waitFor(t, cache.HasSynced, "cache should sync")
	waitFor(t, func() bool {
		return cache.Collection().GetKey("ns/matching") != nil
	}, "a Secret matching the selector should be loaded")

	// Relabel it so the selector no longer matches.
	w.update(secret("ns", "matching", map[string]string{"app": "web"}, "v1"))

	waitFor(t, func() bool {
		return cache.Collection().GetKey("ns/matching") == nil
	}, "a Secret that stops matching the selector should have its payload evicted")
}
