// Package overlaytest checks a plugin's OverlayInputsHash against its
// PerClientClusterOverlay: every field of the backend whose change moves the
// overlay's output must move the hash. A field the overlay reads but the hash
// leaves out is served stale, because the shared base row that carries the
// backend is kept for as long as its hashes compare equal. The check is
// mechanical so a plugin cannot land an overlay that reads more than it
// declares: give it a fixture backend, the mutations a backend can undergo, and
// the clients to evaluate, and it asserts the implication for each.
//
// Only that direction is asserted. A hash that moves when the output does not
// costs one walk of every client over the base collection, which is a
// performance matter, not a correctness one, and is left to review.
package overlaytest

import (
	"context"
	"maps"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// Mutation is one change a backend can undergo, applied to a copy of the
// fixture. Apply must not alias state with the fixture: replace Obj with a deep
// copy (see SetLabel and CloneObject) rather than editing it in place.
type Mutation struct {
	Name  string
	Apply func(backend *ir.BackendObjectIR)
}

// Case is one plugin checked against one fixture.
type Case struct {
	// Plugin is the registration under test. Both PerClientClusterOverlay and
	// OverlayInputsHash must be set; checking one without the other is
	// meaningless and fails the test.
	Plugin sdk.PolicyPlugin
	// Backend is the fixture every mutation starts from. Pick one for which the
	// overlay applies, so that mutations which make it stop applying are
	// observed as output changes.
	Backend ir.BackendObjectIR
	// Base is the shared base cluster an applying overlay mutates a clone of.
	// Nil is replaced by an empty cluster named after the backend.
	Base *envoyclusterv3.Cluster
	// Clients are the clients the overlay is evaluated for. At least one must
	// be given; include one the overlay declines when it gates on the client.
	Clients []ir.UniquelyConnectedClient
	// Mutations are the changes checked. Each is applied on its own to a copy
	// of Backend. At least one must change the overlay's output for some
	// client, or the case proves nothing and fails.
	Mutations []Mutation
	// Kctx is the handler context the overlay fetches through. Nil is replaced
	// by krt.TestingDummyContext{}, which serves synced static collections.
	Kctx krt.HandlerContext
}

// AssertInputsHashCoversOverlay runs the case. Each finding is reported as a
// test error naming the mutation and the client, so a plugin with several
// under-declared fields reports them all in one run.
func AssertInputsHashCoversOverlay(t testing.TB, c Case) {
	t.Helper()
	if c.Plugin.PerClientClusterOverlay == nil {
		t.Fatalf("overlaytest: plugin %q registers no PerClientClusterOverlay", c.Plugin.Name)
	}
	if c.Plugin.OverlayInputsHash == nil {
		t.Fatalf("overlaytest: plugin %q registers PerClientClusterOverlay without OverlayInputsHash", c.Plugin.Name)
	}
	if len(c.Clients) == 0 {
		t.Fatalf("overlaytest: no clients to evaluate")
	}
	if len(c.Mutations) == 0 {
		t.Fatalf("overlaytest: no mutations to check")
	}
	kctx := c.Kctx
	if kctx == nil {
		kctx = krt.TestingDummyContext{}
	}
	base := c.Base
	if base == nil {
		base = &envoyclusterv3.Cluster{Name: c.Backend.ClusterName()}
	}
	ctx := context.Background()

	hashOf := func(backend ir.BackendObjectIR) uint64 {
		first := c.Plugin.OverlayInputsHash(backend)
		if again := c.Plugin.OverlayInputsHash(backend); again != first {
			t.Fatalf("overlaytest: OverlayInputsHash is not deterministic: %d then %d for the same backend", first, again)
		}
		return first
	}
	outputOf := func(ucc ir.UniquelyConnectedClient, backend ir.BackendObjectIR) *envoyclusterv3.Cluster {
		overlay := c.Plugin.PerClientClusterOverlay(kctx, ctx, ucc, backend)
		if overlay == nil {
			return nil
		}
		out := proto.Clone(base).(*envoyclusterv3.Cluster)
		if overlay.Mutate != nil {
			overlay.Mutate(out)
		}
		return out
	}

	fixtureHash := hashOf(c.Backend)
	fixtureOutputs := make([]*envoyclusterv3.Cluster, len(c.Clients))
	for i, ucc := range c.Clients {
		fixtureOutputs[i] = outputOf(ucc, c.Backend)
	}

	anyChanged := false
	for _, m := range c.Mutations {
		mutated := c.Backend
		m.Apply(&mutated)
		hashMoved := hashOf(mutated) != fixtureHash
		for i, ucc := range c.Clients {
			out := outputOf(ucc, mutated)
			if outputsEqual(fixtureOutputs[i], out) {
				continue
			}
			anyChanged = true
			if !hashMoved {
				t.Errorf("overlaytest: mutation %q changed the overlay output for client %s but not OverlayInputsHash; the field it changed is read by the overlay and must be declared",
					m.Name, ucc.ResourceName())
			}
		}
	}
	if !anyChanged {
		t.Errorf("overlaytest: no mutation changed the overlay output for any client; the case is vacuous. Start from a fixture the overlay applies to and mutate the fields it reads")
	}
}

func outputsEqual(a, b *envoyclusterv3.Cluster) bool {
	if a == nil || b == nil {
		return a == b
	}
	return proto.Equal(a, b)
}

// CloneObject deep-copies a backing object so a mutation can edit it without
// touching the fixture. The object must implement runtime.Object, which every
// Kubernetes API type and CRD does.
func CloneObject(t testing.TB, obj metav1.Object) metav1.Object {
	t.Helper()
	ro, ok := obj.(runtime.Object)
	if !ok {
		t.Fatalf("overlaytest: %T does not implement runtime.Object and cannot be cloned", obj)
	}
	cloned, ok := ro.DeepCopyObject().(metav1.Object)
	if !ok {
		t.Fatalf("overlaytest: %T's DeepCopyObject does not return a metav1.Object", obj)
	}
	return cloned
}

// SetLabel is the mutation that sets one label on a copy of the backing object.
func SetLabel(t testing.TB, key, value string) Mutation {
	return Mutation{
		Name: "label " + key + "=" + value,
		Apply: func(backend *ir.BackendObjectIR) {
			obj := CloneObject(t, backend.Obj)
			labels := copyMap(obj.GetLabels())
			labels[key] = value
			obj.SetLabels(labels)
			backend.Obj = obj
		},
	}
}

// RemoveLabel is the mutation that removes one label from a copy of the
// backing object.
func RemoveLabel(t testing.TB, key string) Mutation {
	return Mutation{
		Name: "remove label " + key,
		Apply: func(backend *ir.BackendObjectIR) {
			obj := CloneObject(t, backend.Obj)
			labels := copyMap(obj.GetLabels())
			delete(labels, key)
			obj.SetLabels(labels)
			backend.Obj = obj
		},
	}
}

// SetAnnotation is the mutation that sets one annotation on a copy of the
// backing object.
func SetAnnotation(t testing.TB, key, value string) Mutation {
	return Mutation{
		Name: "annotation " + key + "=" + value,
		Apply: func(backend *ir.BackendObjectIR) {
			obj := CloneObject(t, backend.Obj)
			annotations := copyMap(obj.GetAnnotations())
			annotations[key] = value
			obj.SetAnnotations(annotations)
			backend.Obj = obj
		},
	}
}

// SetResourceVersion is the mutation of a write that changed nothing else: the
// control it is worth including so the case documents that such a write need
// not move the hash.
func SetResourceVersion(t testing.TB, version string) Mutation {
	return Mutation{
		Name: "resourceVersion " + version,
		Apply: func(backend *ir.BackendObjectIR) {
			obj := CloneObject(t, backend.Obj)
			obj.SetResourceVersion(version)
			backend.Obj = obj
		},
	}
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m)+1)
	maps.Copy(out, m)
	return out
}
