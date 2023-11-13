package secrets

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/xds"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewSecretsController(ctx context.Context, mgr manager.Manager, inputChannels *xds.XdsInputChannels) error {
	cb := &controllerBuilder{
		mgr: mgr,
		reconciler: &controllerReconciler{
			cli:           mgr.GetClient(),
			inputChannels: inputChannels,
		},
	}
	return run(ctx, cb.watchSecrets)
}

func run(ctx context.Context, funcs ...func(ctx context.Context) error) error {
	for _, f := range funcs {
		if err := f(ctx); err != nil {
			return err
		}
	}
	return nil
}

type controllerBuilder struct {
	mgr        manager.Manager
	reconciler *controllerReconciler
}

func (c *controllerBuilder) watchSecrets(ctx context.Context) error {
	err := ctrl.NewControllerManagedBy(c.mgr).
		For(&corev1.Secret{}).
		Complete(reconcile.Func(c.reconciler.ReconcileSecrets))
	if err != nil {
		return err
	}
	return nil
}

type controllerReconciler struct {
	cli           client.Client
	inputChannels *xds.XdsInputChannels
}

func (r *controllerReconciler) ReconcileSecrets(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.inputChannels.Kick(ctx)
	return ctrl.Result{}, nil
}
