package common

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	. "github.com/onsi/gomega"
)

func liveNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

// terminatingNs builds a namespace with a deletion timestamp. The fake client refuses objects with
// a deletion timestamp and no finalizers, and a finalizer is what keeps a real one terminating.
func terminatingNs(name string) *corev1.Namespace {
	now := metav1.Now()
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:              name,
		DeletionTimestamp: &now,
		Finalizers:        []string{"test/hold"},
	}}
}

func TestTerminatingNamespace(t *testing.T) {
	cases := []struct {
		name   string
		objs   []client.Object
		lookup []string
		wantNs string
	}{
		{name: "missing namespace", lookup: []string{"a"}},
		{name: "live namespace", objs: []client.Object{liveNamespace("a")}, lookup: []string{"a"}},
		{
			name:   "terminating namespace",
			objs:   []client.Object{liveNamespace("a"), terminatingNs("b")},
			lookup: []string{"a", "b"},
			wantNs: "b",
		},
		{name: "no namespaces", objs: []client.Object{terminatingNs("b")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			c := fake.NewClientBuilder().WithObjects(tc.objs...).Build()
			got, err := terminatingNamespace(t.Context(), c, tc.lookup)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tc.wantNs))
		})
	}
}

func TestWaitForTerminatingNamespacesReturnsAtOnceWhenNothingTerminates(t *testing.T) {
	g := NewWithT(t)
	c := fake.NewClientBuilder().WithObjects(liveNamespace("a")).Build()
	waited, err := waitForTerminatingNamespaces(t.Context(), c, []string{"a", "missing"})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(waited).To(BeZero(), "nothing was terminating, so there is nothing to wait for")
}

func TestWaitForTerminatingNamespacesWaitsUntilGone(t *testing.T) {
	g := NewWithT(t)
	// The namespace stays terminating for the first few polls, then its finalizer is removed, as the
	// namespace controller would once the contents are gone, and the next Get finds nothing.
	gets := 0
	c := fake.NewClientBuilder().WithObjects(terminatingNs("b")).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			gets++
			if gets == 3 {
				ns := &corev1.Namespace{}
				if err := c.Get(ctx, key, ns); err != nil {
					return err
				}
				ns.Finalizers = nil
				if err := c.Update(ctx, ns); err != nil {
					return err
				}
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}).Build()

	waited, err := waitForTerminatingNamespaces(t.Context(), c, []string{"b"})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(gets).To(Equal(3), "should poll until the namespace is gone, then stop")
	g.Expect(waited).To(BeNumerically(">", 0), "a terminating namespace should report a wait")
}

func TestWaitForTerminatingNamespacesTimesOut(t *testing.T) {
	g := NewWithT(t)
	c := fake.NewClientBuilder().WithObjects(terminatingNs("b")).Build()
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	_, err := waitForTerminatingNamespaces(ctx, c, []string{"b"})
	g.Expect(err).To(MatchError(ContainSubstring("namespace b is still terminating")))
}

func TestWaitForTerminatingNamespacesSurfacesGetErrors(t *testing.T) {
	g := NewWithT(t)
	boom := errors.New("boom")
	c := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return boom
		},
	}).Build()
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	_, err := waitForTerminatingNamespaces(ctx, c, []string{"a"})
	g.Expect(err).To(MatchError(ContainSubstring("get namespace a: boom")))
}

func TestNamespacesFromManifests(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "m.yaml")
	g.Expect(os.WriteFile(path, []byte(`
apiVersion: v1
kind: Namespace
metadata:
  name: one
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm
  namespace: one
---
---
apiVersion: v1
kind: Namespace
metadata:
  name: two
`), 0o600)).To(Succeed())
	names, err := namespacesFromManifests(path)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(names).To(Equal([]string{"one", "two"}))
}
