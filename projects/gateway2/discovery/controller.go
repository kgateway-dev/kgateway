package discovery

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/xds"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewDiscoveryController(ctx context.Context, mgr manager.Manager, inputChannels *xds.XdsInputChannels) error {
	cb := &controllerBuilder{
		mgr:        mgr,
		translator: NewTranslator(mgr.GetClient(), inputChannels),
	}
	return run(ctx, cb.watchEndpoints, cb.watchPods, cb.watchServices)
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
	translator Translator
	// reconciler *controllerReconciler
}

func (c *controllerBuilder) watchEndpoints(ctx context.Context) error {
	err := ctrl.NewControllerManagedBy(c.mgr).
		For(&corev1.Endpoints{}).
		Complete(reconcile.Func(c.translator.ReconcileEndpoints))
	if err != nil {
		return err
	}
	return nil
}

func (c *controllerBuilder) watchPods(ctx context.Context) error {
	return ctrl.NewControllerManagedBy(c.mgr).
		Watches(&corev1.Pod{}, handler.Funcs{}). // Empty funcs means that pod will be in the cache but reconcile will never be called.
		Complete(reconcile.Func(c.translator.ReconcilePod))
}

func (c *controllerBuilder) watchServices(ctx context.Context) error {
	err := ctrl.NewControllerManagedBy(c.mgr).
		For(&corev1.Service{}).
		Complete(reconcile.Func(c.translator.ReconcileService))
	if err != nil {
		return err
	}
	return nil
}

type EnvoyState struct {
}

func (e *EnvoyState) updateClusterAndEndpoints() {}
func (e *EnvoyState) removeService()             {}
