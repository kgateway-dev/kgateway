package fake

import (
	"testing"
	"time"

	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/kclient/clienttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestMetadataReadThrough covers the fake-client plumbing the on-demand
// Secret/ConfigMap caches depend on: a metadata-only watch must observe the same
// objects the typed clients do, both for fixtures and for live writes.
func TestMetadataReadThrough(t *testing.T) {
	fixture := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fixture",
			Namespace: "ns",
			Labels:    map[string]string{"team": "infra"},
		},
		Data: map[string][]byte{"tls.crt": []byte("cert")},
	}

	c := NewClient(t, []client.Object{fixture}...)
	md := kclient.NewMetadata(c, gvr.Secret, kclient.Filter{})

	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	c.RunAndWait(stop)

	got := md.List("", labels.Everything())
	if len(got) != 1 {
		t.Fatalf("metadata watch should see the fixture Secret, got %d objects", len(got))
	}
	if got[0].Name != "fixture" || got[0].Namespace != "ns" {
		t.Fatalf("unexpected object %s/%s", got[0].Namespace, got[0].Name)
	}
	if got[0].Labels["team"] != "infra" {
		t.Fatal("labels must survive the metadata conversion, selector refs depend on them")
	}

	// Objects created after the client is running must show up too, otherwise
	// tests that mutate Secrets mid-run would silently diverge.
	secrets := clienttest.NewWriter[*corev1.Secret](t, c)
	secrets.Create(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "later", Namespace: "ns"},
		Data:       map[string][]byte{"k": []byte("v")},
	})

	assertEventually(t, func() bool {
		return md.Get("later", "ns") != nil
	}, "metadata watch should observe a Secret created after start")

	// And deletions must propagate, so the cache can drop the object.
	secrets.Delete("later", "ns")
	assertEventually(t, func() bool {
		return md.Get("later", "ns") == nil
	}, "metadata watch should observe the delete")

	// A metadata object carries no payload; that is the whole point.
	if fixture.Data == nil {
		t.Fatal("fixture should still have data")
	}
	if _, ok := any(md.Get("fixture", "ns")).(*corev1.Secret); ok {
		t.Fatal("metadata watch must not yield typed Secrets")
	}
}

// assertEventually polls cond until it holds or the deadline passes.
func assertEventually(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}
