package controller

import (
	"context"
	"fmt"
	"slices"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	infextv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
)

type inferencePoolReconciler struct {
	cli      client.Client
	scheme   *runtime.Scheme
	deployer *deployer.Deployer
}

func (r *inferencePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, rErr error) {
	log := log.FromContext(ctx).WithValues("inferencepool", req.NamespacedName)
	log.V(1).Info("reconciling request", "request", req)

	finishMetrics := collectReconciliationMetrics("gateway-inferencepool", req)
	defer func() {
		finishMetrics(rErr)
	}()

	pool := new(infextv1a2.InferencePool)
	if err := r.cli.Get(ctx, req.NamespacedName, pool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if pool.GetDeletionTimestamp() != nil {
		log.Info("Removing endpoint picker for InferencePool", "name", pool.Name, "namespace", pool.Namespace)

		// TODO [danehans]: EPP should use role and rolebinding RBAC: https://github.com/kubernetes-sigs/gateway-api-inference-extension/issues/224
		if err := r.cleanupClusterScopedResources(ctx, pool); err != nil {
			return ctrl.Result{}, err
		}

		// Remove the finalizer.
		pool.Finalizers = slices.DeleteFunc(pool.Finalizers, func(s string) bool {
			return s == wellknown.InferencePoolFinalizer
		})

		if err := r.cli.Update(ctx, pool); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Ensure the finalizer is present for the InferencePool.
	if err := EnsureFinalizer(ctx, r.cli, pool); err != nil {
		return ctrl.Result{}, err
	}

	// Use the registered index to list HTTPRoutes that reference this pool.
	var routeList gwv1.HTTPRouteList
	if err := r.cli.List(ctx, &routeList,
		client.InNamespace(pool.Namespace),
		client.MatchingFields{InferencePoolField: pool.Name},
	); err != nil {
		log.Error(err, "failed to list HTTPRoutes referencing InferencePool", "name", pool.Name, "namespace", pool.Namespace)
		return ctrl.Result{}, err
	}

	// If no HTTPRoutes reference the pool, skip reconciliation.
	// TODO [danehans]: The deployer should support switching between an InferencePool/Service backendRef.
	// For example, check if infra exists for the InferencePool and do cleanup, or cache InferencePools that
	// have deployed infra, compare, and remove, or label managed InferencePools and drop the need to ref HTTPRoutes.
	// See the following for details: https://github.com/kubernetes-sigs/gateway-api-inference-extension/issues/489
	if len(routeList.Items) == 0 {
		log.Info("No HTTPRoutes reference this InferencePool; skipping reconciliation")
		return ctrl.Result{}, nil
	}

	objs, err := r.deployer.GetObjsToDeploy(ctx, pool)
	if err != nil {
		return ctrl.Result{}, err
	}
	objs = r.deployer.SetNamespaceAndOwner(pool, objs)

	// TODO [danehans]: Manage inferencepool status conditions.

	// Deploy the endpoint picker resources.
	log.Info("Ensuring endpoint picker is deployed for InferencePool")
	err = r.deployer.DeployObjs(ctx, objs)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.V(1).Info("reconciled request", "request", req)

	return ctrl.Result{}, nil
}

// EnsureFinalizer adds the InferencePool finalizer to the given pool if it’s not already present.
// The deployer requires InferencePools to be finalized to remove cluster-scoped resources.
// This can be removed if the endpoint picker no longer requires cluster-scoped resources.
// See: https://github.com/kubernetes-sigs/gateway-api-inference-extension/issues/224 for details.
func EnsureFinalizer(ctx context.Context, cli client.Client, pool *infextv1a2.InferencePool) error {
	if slices.Contains(pool.Finalizers, wellknown.InferencePoolFinalizer) {
		return nil
	}
	pool.Finalizers = append(pool.Finalizers, wellknown.InferencePoolFinalizer)
	return cli.Update(ctx, pool)
}

// CleanupClusterScopedResources deletes the ClusterRoleBinding for the given pool.
// TODO [danehans]: EPP should use role and rolebinding RBAC: https://github.com/kubernetes-sigs/gateway-api-inference-extension/issues/224
func (r *inferencePoolReconciler) cleanupClusterScopedResources(ctx context.Context, pool *infextv1a2.InferencePool) error {
	// The same release name as in the Helm template.
	releaseName := fmt.Sprintf("%s-endpoint-picker", pool.Name)

	// Delete the ClusterRoleBinding.
	var crb rbacv1.ClusterRoleBinding
	if err := r.cli.Get(ctx, client.ObjectKey{Name: releaseName}, &crb); err == nil {
		if err := r.cli.Delete(ctx, &crb); err != nil {
			return fmt.Errorf("failed to delete ClusterRoleBinding %s: %w", releaseName, err)
		}
	}

	return nil
}
