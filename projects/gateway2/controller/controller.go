package controller

import (
	"context"
	"fmt"
	"reflect"

	"github.com/solo-io/gloo/projects/gateway2/deployer"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func NewBaseGatewayController(ctx context.Context, mgr manager.Manager, gwclass apiv1.ObjectName, release, controllerName string, autoProvision bool, server string, port uint16) error {
	log := log.FromContext(ctx)
	log.V(5).Info("starting controller", "controllerName", controllerName, "gwclass", gwclass)

	controllerBuilder := &controllerBuilder{
		controllerName: controllerName,
		release:        release,
		autoProvision:  autoProvision,
		server:         server,
		port:           port,
		mgr:            mgr,
		gwclass:        gwclass,
		reconciler: &controllerReconciler{
			cli:    mgr.GetClient(),
			scheme: mgr.GetScheme(),
		},
	}

	return run(ctx,
		controllerBuilder.watchGwClass,
		controllerBuilder.watchGw,
		controllerBuilder.watchHttpRoute,
		controllerBuilder.watchReferenceGrant,
		controllerBuilder.watchNamespaces,
		controllerBuilder.addIndexes,
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
	controllerName string
	release        string
	autoProvision  bool
	server         string
	port           uint16
	mgr            manager.Manager
	gwclass        apiv1.ObjectName

	reconciler *controllerReconciler
}

func (c *controllerBuilder) addIndexes(ctx context.Context) error {
	return IterateIndices(func(obj client.Object, field string, indexer client.IndexerFunc) error {
		return c.mgr.GetFieldIndexer().IndexField(ctx, obj, field, indexer)
	})
}

func (c *controllerBuilder) watchGw(ctx context.Context) error {
	// setup a deployer
	log := log.FromContext(ctx)

	log.Info("creating deployer", "ctrlname", c.controllerName, "server", c.server, "port", c.port)
	d, err := deployer.NewDeployer(c.mgr.GetScheme(), c.release, c.controllerName, c.server, c.port)
	if err != nil {
		return err
	}

	gvks, err := d.GetGvksToWatch(ctx)
	if err != nil {
		return err
	}

	buildr := ctrl.NewControllerManagedBy(c.mgr).
		// Don't use WithEventFilter here as it also filters events for Owned objects.
		For(&apiv1.Gateway{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(object client.Object) bool {
			if gw, ok := object.(*apiv1.Gateway); ok {
				return gw.Spec.GatewayClassName == c.gwclass
			}
			return false
		}), predicate.GenerationChangedPredicate{}))

	for _, gvk := range gvks {
		obj, err := c.mgr.GetScheme().New(gvk)
		if err != nil {
			return err
		}
		clientObj, ok := obj.(client.Object)
		if !ok {
			return fmt.Errorf("object %T is not a client.Object", obj)
		}
		log.Info("watching gvk as gateway child", "gvk", gvk)
		// unless its a service, we don't care about the status
		var opts []builder.OwnsOption
		if shouldIgnoreStatusChild(gvk) {
			opts = append(opts, builder.WithPredicates(predicate.GenerationChangedPredicate{}))
		}
		buildr.Owns(clientObj, opts...)
	}

	gwreconciler := &gatewayReconciler{
		cli:           c.mgr.GetClient(),
		scheme:        c.mgr.GetScheme(),
		className:     c.gwclass,
		autoProvision: c.autoProvision,
		deployer:      d,
	}
	err = buildr.Complete(gwreconciler)
	if err != nil {
		return err
	}
	return nil
}

func shouldIgnoreStatusChild(gvk schema.GroupVersionKind) bool {
	// avoid triggering on pod changes that update deployment status
	return gvk.Kind == "Deployment"
}

func (c *controllerBuilder) watchGwClass(ctx context.Context) error {
	return ctrl.NewControllerManagedBy(c.mgr).
		For(&apiv1.GatewayClass{}).
		Complete(reconcile.Func(c.reconciler.ReconcileGatewayClasses))
}

func (c *controllerBuilder) watchHttpRoute(ctx context.Context) error {
	err := ctrl.NewControllerManagedBy(c.mgr).
		For(&apiv1.HTTPRoute{}).
		Complete(reconcile.Func(c.reconciler.ReconcileHttpRoutes))
	if err != nil {
		return err
	}
	return nil
}

func (c *controllerBuilder) watchReferenceGrant(ctx context.Context) error {
	err := ctrl.NewControllerManagedBy(c.mgr).
		For(&apiv1beta1.ReferenceGrant{}).
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

func resolveNs(ns *apiv1.Namespace) string {
	if ns == nil {
		return ""
	}
	return string(*ns)
}

func kind(obj client.Object) apiv1.Kind {
	t := reflect.TypeOf(obj)
	if t.Kind() != reflect.Pointer {
		panic("All types must be pointers to structs.")
	}
	t = t.Elem()
	return apiv1.Kind(t.Name())
}

type controllerReconciler struct {
	cli    client.Client
	scheme *runtime.Scheme
}

func (r *controllerReconciler) ReconcileNamespaces(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileHttpRoutes(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileReferenceGrants(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileGatewayClasses(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("gwclass", req.NamespacedName)

	// if a gateway
	gwclass := &apiv1.GatewayClass{}
	err := r.cli.Get(ctx, req.NamespacedName, gwclass)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("reconciling gateway class")

	// mark it as accepted:
	condition := metav1.Condition{
		Type:               string(apiv1.GatewayClassConditionStatusAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(apiv1.GatewayClassReasonAccepted),
		ObservedGeneration: gwclass.Generation,
		// no need to set LastTransitionTime, it will be set automatically by SetStatusCondition
	}
	meta.SetStatusCondition(&gwclass.Status.Conditions, condition)

	err = r.cli.Status().Update(ctx, gwclass)
	if err != nil {
		return ctrl.Result{}, err
	}
	log.Info("updated gateway class status")

	return ctrl.Result{}, nil
}
