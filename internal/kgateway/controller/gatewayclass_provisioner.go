package controller

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// gatewayClassProvisioner reconciles the provisioned GatewayClass objects
// to ensure they exist.
type gatewayClassProvisioner struct {
	client.Client
	cache.Informers
	// classConfigs maps a GatewayClass name to its desired configuration.
	classConfigs map[string]*ClassInfo
	// controllerName is the name of the controller that is managing the GatewayClass objects.
	controllerName string
	// initialReconcileCh is a channel that is used to trigger initial reconciliation when
	// no GatewayClass objects exist in the cluster.
	initialReconcileCh chan event.TypedGenericEvent[client.Object]
}

var _ reconcile.TypedReconciler[reconcile.Request] = &gatewayClassProvisioner{}
var _ manager.Runnable = &gatewayClassProvisioner{}

// NewGatewayClassProvisioner creates a new GatewayClassProvisioner. It will
// watch for kick events on the channel for initial reconciliation and delete
// events to trigger the re-creation of the GatewayClass. Additionally, it ignores
// update events to allow users to modify the GatewayClasses without this controller
// overwriting them.
func NewGatewayClassProvisioner(mgr ctrl.Manager, controllerName string, classConfigs map[string]*ClassInfo) error {
	initialReconcileCh := make(chan event.TypedGenericEvent[client.Object], 1)
	provisioner := &gatewayClassProvisioner{
		Client:             mgr.GetClient(),
		Informers:          mgr.GetCache(),
		controllerName:     controllerName,
		classConfigs:       classConfigs,
		initialReconcileCh: initialReconcileCh,
	}
	if err := provisioner.SetupWithManager(mgr); err != nil {
		return err
	}
	if err := mgr.Add(provisioner); err != nil {
		return err
	}

	return nil
}

func (r *gatewayClassProvisioner) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("gatewayclass-provisioner").
		For(&apiv1.GatewayClass{}, builder.WithPredicates(predicate.Funcs{
			CreateFunc:  func(e event.CreateEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			UpdateFunc:  func(e event.UpdateEvent) bool { return false },
			GenericFunc: func(e event.GenericEvent) bool { return false },
		})).
		WatchesRawSource(source.Channel(r.initialReconcileCh, handler.TypedEnqueueRequestsFromMapFunc[client.Object, reconcile.Request](
			func(ctx context.Context, o client.Object) []reconcile.Request {
				return []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(o)}}
			},
		))).
		Complete(r)
}

func (r *gatewayClassProvisioner) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("reconciling GatewayClasses", "controllerName", "gatewayclass-provisioner")
	defer log.Info("finished reconciling GatewayClasses", "controllerName", "gatewayclass-provisioner")

	var errs []error
	for name, config := range r.classConfigs {
		if err := r.createGatewayClass(ctx, name, config); err != nil {
			errs = append(errs, err)
			continue
		}
		log.Info("created GatewayClass", "name", name)
	}
	return ctrl.Result{}, errors.Join(errs...)
}

func (r *gatewayClassProvisioner) createGatewayClass(ctx context.Context, name string, config *ClassInfo) error {
	gc := &apiv1.GatewayClass{}
	err := r.Get(ctx, client.ObjectKey{Name: name}, gc)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	gc = &apiv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: config.Annotations,
			Labels:      config.Labels,
		},
		Spec: apiv1.GatewayClassSpec{
			ControllerName: apiv1.GatewayController(r.controllerName),
		},
	}
	if config.Description != "" {
		gc.Spec.Description = ptr.To(config.Description)
	}
	if config.ParametersRef != nil {
		gc.Spec.ParametersRef = config.ParametersRef
	}
	if err := r.Create(ctx, gc); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (r *gatewayClassProvisioner) Start(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("waiting for cache to sync")

	// Wait for cache to sync
	if !r.WaitForCacheSync(ctx) {
		return fmt.Errorf("failed waiting for caches to sync")
	}
	log.Info("caches warm!")

	// Check whether there are any GatewayClass objects in the cluster to determine
	// whether we need to manually trigger initial reconciliation.
	var gcs apiv1.GatewayClassList
	if err := r.List(ctx, &gcs); err != nil {
		return fmt.Errorf("failed to list gatewayclasses: %w", err)
	}
	if len(gcs.Items) > 0 {
		log.Info("gatewayclasses found, skipping initial reconciliation")
		return nil
	}

	log.Info("no gatewayclasses found, triggering initial reconciliation")
	r.initialReconcileCh <- event.TypedGenericEvent[client.Object]{
		Object: &apiv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "manual",
			},
		},
	}

	return nil
}
