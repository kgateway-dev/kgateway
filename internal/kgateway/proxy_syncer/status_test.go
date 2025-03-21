package proxy_syncer

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func TestPolicyStatus(t *testing.T) {
	connPolGK := schema.GroupKind{
		Group: "test",
		Kind:  "ConnectionPolicy",
	}
	tlsPolicyAtt := ir.PolicyAtt{
		GroupKind: wellknown.BackendTLSPolicyGVK.GroupKind(),
		PolicyRef: &ir.AttachedPolicyRef{
			Group:     wellknown.BackendTLSPolicyGVK.Group,
			Kind:      wellknown.BackendTLSPolicyKind,
			Name:      "tls-policy",
			Namespace: "default",
		},
		Errors: []error{
			errors.New("error 1"),
		},
	}
	backends := []ir.BackendObjectIR{
		ir.BackendObjectIR{
			ObjectSource: ir.ObjectSource{
				Group:     "",
				Kind:      "Service",
				Namespace: "default",
				Name:      "reviews",
			},
			AttachedPolicies: ir.AttachedPolicies{
				Policies: map[schema.GroupKind][]ir.PolicyAtt{
					wellknown.BackendTLSPolicyGVK.GroupKind(): []ir.PolicyAtt{
						tlsPolicyAtt,
					},
					connPolGK: []ir.PolicyAtt{
						ir.PolicyAtt{
							GroupKind: connPolGK,
							PolicyRef: &ir.AttachedPolicyRef{
								Group:     connPolGK.Group,
								Kind:      connPolGK.Kind,
								Name:      "conn-policy",
								Namespace: "default",
							},
							Errors: []error{},
						},
					},
				},
			},
		},
		ir.BackendObjectIR{
			ObjectSource: ir.ObjectSource{
				Group:     "",
				Kind:      "Service",
				Namespace: "default",
				Name:      "ratings",
			},
			AttachedPolicies: ir.AttachedPolicies{
				Policies: map[schema.GroupKind][]ir.PolicyAtt{
					wellknown.BackendTLSPolicyGVK.GroupKind(): []ir.PolicyAtt{
						tlsPolicyAtt,
					},
					connPolGK: []ir.PolicyAtt{
						ir.PolicyAtt{
							GroupKind: connPolGK,
							PolicyRef: &ir.AttachedPolicyRef{
								Group:     connPolGK.Group,
								Kind:      connPolGK.Kind,
								Name:      "conn-policy-2",
								Namespace: "default",
							},
							Errors: []error{
								errors.New("error 2"),
							},
						},
					},
				},
			},
		},
	}
	gkPolReport := generateBackendPolicyReport(backends)
	if gkPolReport == nil {
		t.Fatal("GKPolicyReport is nil")
	}
	seenPols := gkPolReport.SeenPolicies
	if len(seenPols) != 2 {
		t.Fatalf("expected 2 unique GKs, found %d", len(seenPols))
	}
	tlsReport, ok := seenPols[wellknown.BackendTLSPolicyGVK.GroupKind().String()]
	if !ok {
		t.Fatal("no policies found for BackendTLSPolicy")
	}
	if len(tlsReport) != 1 {
		t.Fatalf("expected 1 tls policy, found %d", len(tlsReport))
	}
	connReport, ok := seenPols[connPolGK.String()]
	if !ok {
		t.Fatal("no policies found for ConnPolicy")
	}
	if len(connReport) != 2 {
		t.Fatalf("expected 2 conn policies, found %d", len(connReport))
	}
	// TODO: more assertions here
}
