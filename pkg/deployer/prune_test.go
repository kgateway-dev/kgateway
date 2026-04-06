package deployer

import (
	"context"
	"testing"

	"istio.io/istio/pkg/kube"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
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
	var (
		ns         = "test-ns"
		gwName     = "test-gateway"
		ctx        = context.Background()
		deployName = "test-deploy"
		pdbName    = "test-pdb"
		hpaName    = "test-hpa"
	)

	createGateway := func() *gwv1.Gateway {
		gw := &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gwName,
				Namespace: ns,
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: wellknown.DefaultGatewayClassName,
			},
		}
		gw.SetGroupVersionKind(wellknown.GatewayGVK)
		return gw
	}

	createPDB := func(name string, gatewayName string) *policyv1.PodDisruptionBudget {
		pdb := &policyv1.PodDisruptionBudget{
			TypeMeta: metav1.TypeMeta{
				Kind:       wellknown.PodDisruptionBudgetGVK.Kind,
				APIVersion: wellknown.PodDisruptionBudgetGVK.GroupVersion().String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels: map[string]string{
					wellknown.GatewayNameLabel: gatewayName,
				},
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
			},
		}
		return pdb
	}

	createHPA := func(name string, gatewayName string) *autoscalingv2.HorizontalPodAutoscaler {
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			TypeMeta: metav1.TypeMeta{
				Kind:       wellknown.HorizontalPodAutoscalerGVK.Kind,
				APIVersion: wellknown.HorizontalPodAutoscalerGVK.GroupVersion().String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels: map[string]string{
					wellknown.GatewayNameLabel: gatewayName,
				},
			},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					Kind: "Deployment",
					Name: deployName,
				},
				MinReplicas: new(int32(1)),
				MaxReplicas: 10,
			},
		}
		return hpa
	}

	t.Run("prunes PDB when not in desired set", func(t *testing.T) {
		gw := createGateway()
		pdb := createPDB(pdbName, gwName)

		fc := fake.NewClient(t, gw, pdb)
		d := &Deployer{client: fc}

		// Desired set is empty - PDB should be pruned
		err := d.PruneRemovedResources(ctx, gw, []client.Object{})
		if err != nil {
			t.Fatalf("PruneRemovedResources returned error: %v", err)
		}

		// Verify PDB was deleted using dynamic client
		gvr, err := wellknown.GVKToGVR(wellknown.PodDisruptionBudgetGVK)
		if err != nil {
			t.Fatalf("failed to get GVR: %v", err)
		}
		list, err := fc.Dynamic().Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("failed to list PDBs: %v", err)
		}
		if len(list.Items) != 0 {
			t.Errorf("expected 0 PDBs, got %d", len(list.Items))
		}
	})

	t.Run("keeps PDB when in desired set", func(t *testing.T) {
		gw := createGateway()
		pdb := createPDB(pdbName, gwName)

		fc := fake.NewClient(t, gw, pdb)
		d := &Deployer{client: fc}

		// PDB is in desired set - should be kept
		desiredPDB := createPDB(pdbName, gwName)
		err := d.PruneRemovedResources(ctx, gw, []client.Object{desiredPDB})
		if err != nil {
			t.Fatalf("PruneRemovedResources returned error: %v", err)
		}

		// Verify PDB still exists using dynamic client
		gvr, err := wellknown.GVKToGVR(wellknown.PodDisruptionBudgetGVK)
		if err != nil {
			t.Fatalf("failed to get GVR: %v", err)
		}
		list, err := fc.Dynamic().Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("failed to list PDBs: %v", err)
		}
		if len(list.Items) != 1 {
			t.Errorf("expected 1 PDB, got %d", len(list.Items))
		}
		if list.Items[0].GetName() != pdbName {
			t.Errorf("expected PDB name %q, got %q", pdbName, list.Items[0].GetName())
		}
	})

	t.Run("skips resources belonging to a different Gateway", func(t *testing.T) {
		gw := createGateway()
		// PDB labeled for a different Gateway
		pdb := createPDB(pdbName, "other-gateway")

		fc := fake.NewClient(t, gw, pdb)
		d := &Deployer{client: fc}

		// Empty desired set, but PDB belongs to a different Gateway
		err := d.PruneRemovedResources(ctx, gw, []client.Object{})
		if err != nil {
			t.Fatalf("PruneRemovedResources returned error: %v", err)
		}

		// Verify PDB was NOT deleted (different gateway label)
		gvr, err := wellknown.GVKToGVR(wellknown.PodDisruptionBudgetGVK)
		if err != nil {
			t.Fatalf("failed to get GVR: %v", err)
		}
		list, err := fc.Dynamic().Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("failed to list PDBs: %v", err)
		}
		if len(list.Items) != 1 {
			t.Errorf("expected 1 PDB (not deleted), got %d", len(list.Items))
		}
	})

	t.Run("prunes multiple resources in one call", func(t *testing.T) {
		gw := createGateway()
		pdb := createPDB(pdbName, gwName)
		hpa := createHPA(hpaName, gwName)

		fc := fake.NewClient(t, gw, pdb, hpa)
		d := &Deployer{client: fc}

		// Empty desired set - both should be pruned
		err := d.PruneRemovedResources(ctx, gw, []client.Object{})
		if err != nil {
			t.Fatalf("PruneRemovedResources returned error: %v", err)
		}

		// Verify both were deleted
		pdbGVR, err := wellknown.GVKToGVR(wellknown.PodDisruptionBudgetGVK)
		if err != nil {
			t.Fatalf("failed to get PDB GVR: %v", err)
		}
		pdbList, err := fc.Dynamic().Resource(pdbGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("failed to list PDBs: %v", err)
		}
		if len(pdbList.Items) != 0 {
			t.Errorf("expected 0 PDBs, got %d", len(pdbList.Items))
		}

		hpaGVR, err := wellknown.GVKToGVR(wellknown.HorizontalPodAutoscalerGVK)
		if err != nil {
			t.Fatalf("failed to get HPA GVR: %v", err)
		}
		hpaList, err := fc.Dynamic().Resource(hpaGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("failed to list HPAs: %v", err)
		}
		if len(hpaList.Items) != 0 {
			t.Errorf("expected 0 HPAs, got %d", len(hpaList.Items))
		}
	})

	t.Run("prunes some resources while keeping others", func(t *testing.T) {
		gw := createGateway()
		pdb := createPDB(pdbName, gwName)
		hpa := createHPA(hpaName, gwName)

		fc := fake.NewClient(t, gw, pdb, hpa)
		d := &Deployer{client: fc}

		// Only PDB in desired set - HPA should be pruned
		desiredPDB := createPDB(pdbName, gwName)
		err := d.PruneRemovedResources(ctx, gw, []client.Object{desiredPDB})
		if err != nil {
			t.Fatalf("PruneRemovedResources returned error: %v", err)
		}

		// Verify PDB still exists
		pdbGVR, err := wellknown.GVKToGVR(wellknown.PodDisruptionBudgetGVK)
		if err != nil {
			t.Fatalf("failed to get PDB GVR: %v", err)
		}
		pdbList, err := fc.Dynamic().Resource(pdbGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("failed to list PDBs: %v", err)
		}
		if len(pdbList.Items) != 1 {
			t.Errorf("expected 1 PDB, got %d", len(pdbList.Items))
		}

		// Verify HPA was deleted
		hpaGVR, err := wellknown.GVKToGVR(wellknown.HorizontalPodAutoscalerGVK)
		if err != nil {
			t.Fatalf("failed to get HPA GVR: %v", err)
		}
		hpaList, err := fc.Dynamic().Resource(hpaGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("failed to list HPAs: %v", err)
		}
		if len(hpaList.Items) != 0 {
			t.Errorf("expected 0 HPAs, got %d", len(hpaList.Items))
		}
	})

	t.Run("handles no existing resources gracefully", func(t *testing.T) {
		gw := createGateway()

		fc := fake.NewClient(t, gw)
		d := &Deployer{client: fc}

		// No resources exist, empty desired set
		err := d.PruneRemovedResources(ctx, gw, []client.Object{})
		if err != nil {
			t.Fatalf("PruneRemovedResources returned error: %v", err)
		}
	})

	t.Run("handles empty desired set", func(t *testing.T) {
		gw := createGateway()
		pdb := createPDB(pdbName, gwName)
		hpa := createHPA(hpaName, gwName)

		fc := fake.NewClient(t, gw, pdb, hpa)
		d := &Deployer{client: fc}

		// All resources should be pruned with empty desired set
		err := d.PruneRemovedResources(ctx, gw, []client.Object{})
		if err != nil {
			t.Fatalf("PruneRemovedResources returned error: %v", err)
		}

		// Verify all were deleted
		pdbGVR, err := wellknown.GVKToGVR(wellknown.PodDisruptionBudgetGVK)
		if err != nil {
			t.Fatalf("failed to get PDB GVR: %v", err)
		}
		pdbList, err := fc.Dynamic().Resource(pdbGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("failed to list PDBs: %v", err)
		}
		if len(pdbList.Items) != 0 {
			t.Errorf("expected 0 PDBs, got %d", len(pdbList.Items))
		}

		hpaGVR, err := wellknown.GVKToGVR(wellknown.HorizontalPodAutoscalerGVK)
		if err != nil {
			t.Fatalf("failed to get HPA GVR: %v", err)
		}
		hpaList, err := fc.Dynamic().Resource(hpaGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("failed to list HPAs: %v", err)
		}
		if len(hpaList.Items) != 0 {
			t.Errorf("expected 0 HPAs, got %d", len(hpaList.Items))
		}
	})
}

func TestPruneRemovedResourcesLongGatewayName(t *testing.T) {
	var (
		ns  = "test-ns"
		ctx = context.Background()
	)
	// Gateway name > 63 chars - will be truncated in labels
	longGwName := "very-long-gateway-name-for-testing-80-char-limit-exactly-this-many-chars"
	safeGwName := kubeutils.SafeGatewayLabelValue(longGwName)

	createGateway := func(name string) *gwv1.Gateway {
		gw := &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: wellknown.DefaultGatewayClassName,
			},
		}
		gw.SetGroupVersionKind(wellknown.GatewayGVK)
		return gw
	}

	createPDB := func(name string, gatewayName string) *policyv1.PodDisruptionBudget {
		pdb := &policyv1.PodDisruptionBudget{
			TypeMeta: metav1.TypeMeta{
				Kind:       wellknown.PodDisruptionBudgetGVK.Kind,
				APIVersion: wellknown.PodDisruptionBudgetGVK.GroupVersion().String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels: map[string]string{
					wellknown.GatewayNameLabel: gatewayName,
				},
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
			},
		}
		return pdb
	}

	t.Run("prunes resources with long gateway name using safe label value", func(t *testing.T) {
		gw := createGateway(longGwName)
		// PDB labeled with the SAFE name (truncated + hash), not the original long name
		pdb := createPDB("test-pdb", safeGwName)

		fc := fake.NewClient(t, gw, pdb)
		d := &Deployer{client: fc}

		// Desired set is empty - PDB should be pruned because the label selector
		// uses SafeGatewayLabelValue(longGwName) which equals safeGwName
		err := d.PruneRemovedResources(ctx, gw, []client.Object{})
		if err != nil {
			t.Fatalf("PruneRemovedResources returned error: %v", err)
		}

		// Verify PDB was deleted
		gvr, err := wellknown.GVKToGVR(wellknown.PodDisruptionBudgetGVK)
		if err != nil {
			t.Fatalf("failed to get GVR: %v", err)
		}
		list, err := fc.Dynamic().Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("failed to list PDBs: %v", err)
		}
		if len(list.Items) != 0 {
			t.Errorf("expected 0 PDBs, got %d", len(list.Items))
		}
	})

	t.Run("keeps resources when desired set includes them with safe label value", func(t *testing.T) {
		gw := createGateway(longGwName)
		pdb := createPDB("test-pdb", safeGwName)

		fc := fake.NewClient(t, gw, pdb)
		d := &Deployer{client: fc}

		// Desired set includes PDB with the safe label value - should be kept
		desiredPDB := createPDB("test-pdb", safeGwName)
		err := d.PruneRemovedResources(ctx, gw, []client.Object{desiredPDB})
		if err != nil {
			t.Fatalf("PruneRemovedResources returned error: %v", err)
		}

		// Verify PDB still exists
		gvr, err := wellknown.GVKToGVR(wellknown.PodDisruptionBudgetGVK)
		if err != nil {
			t.Fatalf("failed to get GVR: %v", err)
		}
		list, err := fc.Dynamic().Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("failed to list PDBs: %v", err)
		}
		if len(list.Items) != 1 {
			t.Errorf("expected 1 PDB, got %d", len(list.Items))
		}
	})
}
