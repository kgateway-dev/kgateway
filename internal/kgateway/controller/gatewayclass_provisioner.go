package controller

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// gatewayClassProvisioner reconciles the provisioned GatewayClass objects
// to ensure they exist.
type gatewayClassProvisioner struct {
	client.Client
	// classConfigs maps a GatewayClass name to its desired configuration.
	classConfigs map[string]*ClassInfo
	// controllerName is the name of the controller that is managing the GatewayClass objects.
	controllerName string
	// kick is a channel that is used to trigger initial reconciliation when
	// no GatewayClass objects exist in the cluster.
	kick chan event.TypedGenericEvent[client.Object]
}

var _ reconcile.TypedReconciler[reconcile.Request] = &gatewayClassProvisioner{}

// NewGatewayClassProvisioner creates a new GatewayClassProvisioner. It will
// watch for kick events on the channel for initial reconciliation and delete
// events to trigger the re-creation of the GatewayClass. Additionally, it ignores
// update events to allow users to modify the GatewayClasses without this controller
// overwriting them.
func NewGatewayClassProvisioner(
	mgr ctrl.Manager,
	controllerName string,
	classConfigs map[string]*ClassInfo,
	ch chan event.TypedGenericEvent[client.Object],
) error {
	r := &gatewayClassProvisioner{
		Client:         mgr.GetClient(),
		controllerName: controllerName,
		classConfigs:   classConfigs,
		kick:           ch,
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("gatewayclass-provisioner").
		For(&apiv1.GatewayClass{}, builder.WithPredicates(predicate.Funcs{
			CreateFunc:  func(e event.CreateEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			UpdateFunc:  func(e event.UpdateEvent) bool { return false },
			GenericFunc: func(e event.GenericEvent) bool { return false },
		})).
		WatchesRawSource(source.Channel(r.kick, handler.TypedEnqueueRequestsFromMapFunc[client.Object, reconcile.Request](
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
