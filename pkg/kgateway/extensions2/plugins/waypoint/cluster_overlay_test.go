package waypoint

import (
	"context"
	"testing"

	istioannot "istio.io/api/annotation"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// TestClusterOverlay_NilForNonAmbientClient pins the cheap UCC-only fast path:
// a client without the ambient-redirection annotation can never be affected by
// the waypoint cluster overlay, so clusterOverlay must return nil before doing
// any of the expensive Gateway/service lookups. This is the gate that keeps the
// per-client cluster collection sparse for the dominant (non-ambient) client.
func TestClusterOverlay_NilForNonAmbientClient(t *testing.T) {
	// commonCols is intentionally left nil: a correct fast path returns before
	// dereferencing it, so a panic here would mean the gate regressed.
	p := &PerClientProcessor{}
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Group: "", Kind: "Service", Namespace: "ns", Name: "svc"}, 80, "", "")

	cases := map[string]map[string]string{
		"no ambient label":      {"some": "label"},
		"ambient label not set": nil,
		"ambient label != enabled": {
			istioannot.AmbientRedirection.Name: "disabled",
		},
	}
	for name, labels := range cases {
		t.Run(name, func(t *testing.T) {
			ucc := ir.NewUniquelyConnectedClient("role", "ns", labels, ir.PodLocality{})
			got := p.clusterOverlay(krt.TestingDummyContext{}, context.Background(), ucc, backend)
			if got != nil {
				t.Fatalf("expected nil overlay for non-ambient client, got %#v", got)
			}
		})
	}
}
