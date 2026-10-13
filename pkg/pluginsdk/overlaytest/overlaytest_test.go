package overlaytest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// recordingTB captures the errors the harness reports instead of failing the
// real test, so the harness's own verdicts can be asserted.
type recordingTB struct {
	testing.TB
	errors []string
}

func (r *recordingTB) Helper() {}
func (r *recordingTB) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

// labelOverlay applies when the backing object carries gate=on and inlines
// the value of the "shape" label into the cluster; it therefore reads two
// labels, and declares whichever the test passes.
func labelOverlay(declared ...string) sdk.PolicyPlugin {
	return sdk.PolicyPlugin{
		Name: "label-overlay",
		PerClientClusterOverlay: func(_ krt.HandlerContext, _ context.Context, _ ir.UniquelyConnectedClient, in ir.BackendObjectIR) *sdk.ClusterOverlay {
			labels := in.Obj.GetLabels()
			if labels["gate"] != "on" {
				return nil
			}
			shape := labels["shape"]
			return &sdk.ClusterOverlay{Mutate: func(out *envoyclusterv3.Cluster) { out.AltStatName = shape }}
		},
		OverlayInputsHash: func(in ir.BackendObjectIR) uint64 {
			var h uint64
			for _, key := range declared {
				h = h*31 + utils.HashString(key+"="+in.Obj.GetLabels()[key])
			}
			return h
		},
	}
}

func fixtureBackend() ir.BackendObjectIR {
	b := ir.NewBackendObjectIR(ir.ObjectSource{Group: "", Kind: "Service", Namespace: "ns", Name: "svc"}, 80, "", "")
	b.Obj = &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Namespace: "ns", Name: "svc", UID: "uid", ResourceVersion: "1",
		Labels: map[string]string{"gate": "on", "shape": "round"},
	}}
	return b
}

func run(t *testing.T, plugin sdk.PolicyPlugin, mutations ...Mutation) []string {
	t.Helper()
	rec := &recordingTB{TB: t}
	AssertInputsHashCoversOverlay(rec, Case{
		Plugin:    plugin,
		Backend:   fixtureBackend(),
		Clients:   []ir.UniquelyConnectedClient{ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})},
		Mutations: mutations,
	})
	return rec.errors
}

func TestAssertInputsHashCoversOverlay(t *testing.T) {
	fixture := fixtureBackend()
	if fixture.Obj.GetLabels()["gate"] != "on" {
		t.Fatal("fixture must be one the overlay applies to")
	}

	t.Run("fully declared passes", func(t *testing.T) {
		errs := run(t, labelOverlay("gate", "shape"),
			SetLabel(t, "gate", "off"),
			SetLabel(t, "shape", "square"),
			SetAnnotation(t, "unread", "x"),
			SetResourceVersion(t, "2"),
		)
		if len(errs) != 0 {
			t.Fatalf("expected no findings, got %q", errs)
		}
	})

	t.Run("an undeclared read is reported once per client that observed it", func(t *testing.T) {
		errs := run(t, labelOverlay("gate"),
			SetLabel(t, "gate", "off"),
			SetLabel(t, "shape", "square"),
		)
		if len(errs) != 1 || !strings.Contains(errs[0], "shape=square") {
			t.Fatalf("expected one finding naming the shape mutation, got %q", errs)
		}
	})

	t.Run("a vacuous case is reported", func(t *testing.T) {
		errs := run(t, labelOverlay("gate", "shape"),
			SetAnnotation(t, "unread", "x"),
			SetResourceVersion(t, "2"),
		)
		if len(errs) != 1 || !strings.Contains(errs[0], "vacuous") {
			t.Fatalf("expected the vacuity finding, got %q", errs)
		}
	})

	t.Run("mutations do not alias the fixture", func(t *testing.T) {
		backend := fixtureBackend()
		mutated := backend
		SetLabel(t, "shape", "square").Apply(&mutated)
		if backend.Obj.GetLabels()["shape"] != "round" {
			t.Fatal("SetLabel edited the fixture's object in place")
		}
		if mutated.Obj.GetLabels()["shape"] != "square" {
			t.Fatal("SetLabel did not apply to the copy")
		}
	})
}
