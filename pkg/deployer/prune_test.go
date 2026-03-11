package deployer

import (
	"context"
	"testing"

	"istio.io/istio/pkg/kube"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func init() {
	// Register VPA list kind in the fake scheme so the fake dynamic client
	// can list VPA resources without panicking. VPA is a custom resource
	// not included in the standard Kubernetes scheme.
	kube.FakeIstioScheme.AddKnownTypeWithName(
		wellknown.VerticalPodAutoscalerGVK.GroupVersion().WithKind("VerticalPodAutoscalerList"),
		&unstructured.UnstructuredList{},
	)
}

func TestPruneRemovedResources(t *testing.T) {
	const (
		namespace = "default"
	)
	var (
		gwUID    = types.UID("gateway-uid-123")
		otherUID = types.UID("other-uid-456")
	)

	tests := []struct {
		name           string
		existingPDBs   []*policyv1.PodDisruptionBudget
		desiredObjs    []client.Object
		expectDeleted  []string
		expectRetained []string
	}{
		{
			name: "deletes owned PDB not in desired set",
			existingPDBs: []*policyv1.PodDisruptionBudget{
				testPDB("my-pdb", gwUID),
			},
			desiredObjs:   nil,
			expectDeleted: []string{"my-pdb"},
		},
		{
			name: "retains PDB that is in desired set",
			existingPDBs: []*policyv1.PodDisruptionBudget{
				testPDB("my-pdb", gwUID),
			},
			desiredObjs: []client.Object{
				testPDB("my-pdb", gwUID),
			},
			expectRetained: []string{"my-pdb"},
		},
		{
			name: "does not delete PDB owned by a different UID",
			existingPDBs: []*policyv1.PodDisruptionBudget{
				testPDB("other-pdb", otherUID),
			},
			desiredObjs:    nil,
			expectRetained: []string{"other-pdb"},
		},
		{
			name: "mixed: deletes orphaned owned, retains unowned and desired",
			existingPDBs: []*policyv1.PodDisruptionBudget{
				testPDB("orphaned-pdb", gwUID),
				testPDB("kept-pdb", gwUID),
				testPDB("unowned-pdb", otherUID),
			},
			desiredObjs: []client.Object{
				testPDB("kept-pdb", gwUID),
			},
			expectDeleted:  []string{"orphaned-pdb"},
			expectRetained: []string{"kept-pdb", "unowned-pdb"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var initObjs []client.Object
			for _, pdb := range tt.existingPDBs {
				initObjs = append(initObjs, pdb)
			}

			fakeClient := fake.NewClient(t, initObjs...)

			d := &Deployer{
				client: fakeClient,
			}

			ctx := context.Background()
			if err := d.PruneRemovedResources(ctx, gwUID, namespace, tt.desiredObjs); err != nil {
				t.Fatalf("PruneRemovedResources returned error: %v", err)
			}

			pdbDynClient := fakeClient.Dynamic().Resource(wellknown.PodDisruptionBudgetGVR).Namespace(namespace)
			remaining, err := pdbDynClient.List(ctx, metav1.ListOptions{})
			if err != nil {
				t.Fatalf("failed to list PDBs: %v", err)
			}

			remainingNames := make(map[string]bool)
			for _, item := range remaining.Items {
				remainingNames[item.GetName()] = true
			}

			for _, name := range tt.expectDeleted {
				if remainingNames[name] {
					t.Errorf("expected PDB %q to be deleted, but it still exists", name)
				}
			}
			for _, name := range tt.expectRetained {
				if !remainingNames[name] {
					t.Errorf("expected PDB %q to be retained, but it was deleted", name)
				}
			}
		})
	}
}

func testPDB(name string, ownerUID types.UID) *policyv1.PodDisruptionBudget {
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: wellknown.GatewayGVK.GroupVersion().String(),
					Kind:       wellknown.GatewayGVK.Kind,
					Name:       "my-gateway",
					UID:        ownerUID,
				},
			},
		},
	}
	pdb.SetGroupVersionKind(wellknown.PodDisruptionBudgetGVK)
	return pdb
}
