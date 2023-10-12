package controller

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	api "sigs.k8s.io/gateway-api/apis/v1beta1"
)



func newBaseGatewayController(ctx context.Context, mgr manager.Manager, gwclass api.ObjectName) error {

	controllerBuilder := &controllerBuilder{
		mgr:     mgr,
		gwclass: gwclass,
		reconciler: &gatewayReconciler{
			Client: mgr.GetClient(),
			scheme: mgr.GetScheme(),
		},
	}

	return run(ctx,
		controllerBuilder.watchGwClass,
		controllerBuilder.watchGw,
		controllerBuilder.watchHttpRoute,
		controllerBuilder.watchReferenceGrant,
		controllerBuilder.watchNamespaces,
	)

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
	mgr     manager.Manager
	gwclass api.ObjectName

	reconciler *gatewayReconciler
}

func (c *controllerBuilder) addIndexes(ctx context.Context) error {
	return IterateIndices(func(obj client.Object, field string, indexer client.IndexerFunc) error {
		return c.mgr.GetFieldIndexer().IndexField(ctx, obj, field, indexer)
	})
}

func (c *controllerBuilder) watchGw(ctx context.Context) error {
	return ctrl.NewControllerManagedBy(c.mgr).
		For(&api.Gateway{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			if gw, ok := object.(*api.Gateway); ok {
				return gw.Spec.GatewayClassName == c.gwclass
			}
			return false
		})).Complete(reconcile.Func(c.reconciler.ReconcileGateway))

}

func (c *controllerBuilder) watchGwClass(ctx context.Context) error {
	return ctrl.NewControllerManagedBy(c.mgr).
		For(&api.GatewayClass{}).
		Complete(reconcile.Func(c.reconciler.ReconcileGatewayClasses))
}

func (c *controllerBuilder) watchHttpRoute(ctx context.Context) error {
	err := ctrl.NewControllerManagedBy(c.mgr).
		For(&api.HTTPRoute{}).
		Complete(reconcile.Func(c.reconciler.ReconcileHttpRoutes))
	if err != nil {
		return err
	}
	return nil
}

func (c *controllerBuilder) watchReferenceGrant(ctx context.Context) error {
	err := ctrl.NewControllerManagedBy(c.mgr).
		For(&api.ReferenceGrant{}).
		Complete(reconcile.Func(c.reconciler.ReconcileReferenceGrants))
	if err != nil {
		return err
	}
	return nil
}

func (c *controllerBuilder) watchNamespaces(ctx context.Context) error {
	err := ctrl.NewControllerManagedBy(c.mgr).
		For(&corev1.Namespace{}).
		Complete(reconcile.Func(c.reconciler.ReconcileNamespaces))
	if err != nil {
		return err
	}
	return nil
}

func resolveNs(ns *api.Namespace) string {
	if ns == nil {
		return ""
	}
	return string(*ns)
}

func kind(obj client.Object) api.Kind {
	t := reflect.TypeOf(obj)
	if t.Kind() != reflect.Pointer {
		panic("All types must be pointers to structs.")
	}
	t = t.Elem()
	return api.Kind(t.Name())
}

type gatewayReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

func (r *gatewayReconciler) ReconcileNamespaces(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *gatewayReconciler) ReconcileHttpRoutes(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *gatewayReconciler) ReconcileReferenceGrants(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *gatewayReconciler) ReconcileGatewayClasses(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *gatewayReconciler) ReconcileGateway(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("gw", req.NamespacedName)
	log.V(1).Info("reconciling request", req)

	var gwlist api.GatewayList
	if err := r.List(ctx, &gwlist); err != nil {
		log.Error(err, "unable to get gateways")
		return ctrl.Result{}, err
	}

	// Not sure we have what to do here, if anything at all. The gateways here should be of our class due to the event filter.

	return ctrl.Result{}, nil
}
