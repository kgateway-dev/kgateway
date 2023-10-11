package controller

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	api "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	httpRouteTargetField = "http-route-target"
	referenceGrantFrom   = "ref-grant-from"
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
	if err := c.mgr.GetFieldIndexer().IndexField(ctx, &api.HTTPRoute{}, httpRouteTargetField, httpRouteToTargetIndex); err != nil {
		return err
	}
	return nil
}

func httpRouteToTargetIndex(obj client.Object) []string {
	hr := obj.(*api.HTTPRoute)
	var parents []string
	for _, pRef := range hr.Spec.ParentRefs {
		if pRef.Kind == nil || *pRef.Kind == kind(&api.Gateway{}) {
			ns := resolveNs(pRef.Namespace)
			if ns == "" {
				ns = hr.Namespace
			}
			if ns == "" {
				continue
			}
			nns := types.NamespacedName{
				Namespace: ns,
				Name:      string(pRef.Name),
			}
			parents = append(parents, nns.String())
		}
	}
	return parents
}

func (c *controllerBuilder) watchReferenceGrant(ctx context.Context) error {
	err := ctrl.NewControllerManagedBy(c.mgr).
		For(&api.ReferenceGrant{}).
		Complete(reconcile.Func(c.reconciler.ReconcileReferenceGrants))
	if err != nil {
		return err
	}
	if err := c.mgr.GetFieldIndexer().IndexField(ctx, &api.ReferenceGrant{}, referenceGrantFrom, refGrantFrom); err != nil {
		return err
	}
	return nil
}

func refGrantFrom(obj client.Object) []string {
	rg := obj.(*api.ReferenceGrant)
	var ns []string
	for _, from := range rg.Spec.From {
		if from.Namespace != "" {
			ns = append(ns, string(from.Namespace))
		}
	}
	return ns
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
