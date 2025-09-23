package controller

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
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

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
)

// gatewayClassProvisioner reconciles the provisioned GatewayClass objects
// to ensure they exist.
type gatewayClassProvisioner struct {
	client.Client
	cache.Informers
	// classConfigs maps a GatewayClass name to its desired configuration.
	classConfigs map[string]*deployer.GatewayClassInfo
	// initialReconcileCh is a channel that is used to trigger initial reconciliation when
	// no GatewayClass objects exist in the cluster.
	initialReconcileCh chan event.TypedGenericEvent[client.Object]
	// defaultControllerName is the name of the default controller that is managing the GatewayClass objects (kgateway).
	defaultControllerName string
}

var _ reconcile.TypedReconciler[reconcile.Request] = &gatewayClassProvisioner{}
var _ manager.LeaderElectionRunnable = &gatewayClassProvisioner{}

// NewGatewayClassProvisioner creates a new GatewayClassProvisioner. It will
// watch for kick events on the channel for initial reconciliation and delete
// events to trigger the re-creation of the GatewayClass. Additionally, it ignores
// update events to allow users to modify the GatewayClasses without this controller
// overwriting them.
func NewGatewayClassProvisioner(mgr ctrl.Manager, defaultControllerName string, classConfigs map[string]*deployer.GatewayClassInfo) error {
	initialReconcileCh := make(chan event.TypedGenericEvent[client.Object], 1)
	provisioner := &gatewayClassProvisioner{
		Client:                mgr.GetClient(),
		Informers:             mgr.GetCache(),
		defaultControllerName: defaultControllerName,
		classConfigs:          classConfigs,
		initialReconcileCh:    initialReconcileCh,
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
		For(&apiv1.GatewayClass{}).
		Named("gatewayclass-provisioner").
		WithEventFilter(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			gc, ok := obj.(*apiv1.GatewayClass)
			if !ok {
				return false
			}
			// only reconcile GatewayClass objects that are managed by this controller
			// the controller is determined by the GatewayClassInfo tied to the class name, or the default controller if none is set
			classConfig, exists := r.classConfigs[gc.Name]
			if !exists {
				return gc.Spec.ControllerName == apiv1.GatewayController(r.defaultControllerName)
			}
			return gc.Spec.ControllerName == apiv1.GatewayController(classConfig.ControllerName)
		})).
		WatchesRawSource(source.Channel(r.initialReconcileCh, handler.TypedEnqueueRequestsFromMapFunc(
			func(ctx context.Context, o client.Object) []reconcile.Request {
				return []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(o)}}
			},
		))).
		Complete(r)
}

func (r *gatewayClassProvisioner) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, rErr error) {
	log := log.FromContext(ctx)
	log.Info("reconciling GatewayClasses", "controllerName", "gatewayclass-provisioner")
	defer log.Info("finished reconciling GatewayClasses", "controllerName", "gatewayclass-provisioner")

	finishMetrics := collectReconciliationMetrics("gatewayclass-provisioner", req)
	defer func() {
		finishMetrics(rErr)
	}()

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

func (r *gatewayClassProvisioner) createGatewayClass(ctx context.Context, name string, config *deployer.GatewayClassInfo) error {
	gc := &apiv1.GatewayClass{}
	err := r.Get(ctx, client.ObjectKey{Name: name}, gc)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	controllerName := r.defaultControllerName
	if r.classConfigs[name] != nil && r.classConfigs[name].ControllerName != "" {
		controllerName = r.classConfigs[name].ControllerName
	}
	gc = &apiv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: config.Annotations,
			Labels:      config.Labels,
		},
		Spec: apiv1.GatewayClassSpec{
			ControllerName: apiv1.GatewayController(controllerName),
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
	var missing bool
	for _, gc := range gcs.Items {
		if _, exists := r.classConfigs[gc.Name]; !exists {
			missing = true
			break
		}
	}
	if len(gcs.Items) > 0 && !missing {
		log.Info("all required gatewayclasses found, skipping initial reconciliation")
		return nil
	}

	log.Info("some required gatewayclasses missing, triggering initial reconciliation")
	r.initialReconcileCh <- event.TypedGenericEvent[client.Object]{
		Object: &apiv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "manual",
			},
			Spec: apiv1.GatewayClassSpec{
				ControllerName: wellknown.DefaultGatewayControllerName,
			},
		},
	}

	return nil
}

// NeedLeaderElection returns true to ensure that the gatewayClassProvisioner runs only on the leader
func (r *gatewayClassProvisioner) NeedLeaderElection() bool {
	return true
}
