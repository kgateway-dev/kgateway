package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	infextv1a1 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha1"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/deployer"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

const (
	// field name used for indexing
	GatewayParamsField = "gateway-params"
)

// TODO [danehans]: Refactor so controller config is organized into shared and Gateway/InferencePool-specific controllers.
type GatewayConfig struct {
	Mgr manager.Manager

	OurGateway func(gw *apiv1.Gateway) bool

	Dev            bool
	ControllerName string
	AutoProvision  bool

	ControlPlane            deployer.ControlPlaneInfo
	IstioIntegrationEnabled bool
	Aws                     *deployer.AwsInfo
}

func NewBaseGatewayController(ctx context.Context, cfg GatewayConfig) error {
	log := log.FromContext(ctx)
	log.V(5).Info("starting gateway controller", "controllerName", cfg.ControllerName)

	controllerBuilder := &controllerBuilder{
		cfg: cfg,
		reconciler: &controllerReconciler{
			cli:    cfg.Mgr.GetClient(),
			scheme: cfg.Mgr.GetScheme(),
		},
	}

	return run(ctx,
		controllerBuilder.watchGwClass,
		controllerBuilder.watchGw,
		controllerBuilder.addGwParamsIndexes,
	)
}

type InferencePoolConfig struct {
	Mgr            manager.Manager
	ControllerName string
	InferenceExt   *deployer.InferenceExtInfo
	// OurPool determines whether a given InferencePool is managed by this controller.
	OurPool func(pool *infextv1a1.InferencePool) bool
}

func NewBaseInferencePoolController(ctx context.Context, poolCfg *InferencePoolConfig, gwCfg *GatewayConfig) error {
	log := log.FromContext(ctx)
	log.V(5).Info("starting inferencepool controller", "controllerName", poolCfg.ControllerName)

	// Set default OurPool if not provided.
	if poolCfg.OurPool == nil {
		poolCfg.OurPool = func(pool *infextv1a1.InferencePool) bool {
			// List HTTPRoutes in the same namespace.
			var routes apiv1.HTTPRouteList
			if err := poolCfg.Mgr.GetClient().List(ctx, &routes, client.InNamespace(pool.Namespace)); err != nil {
				log.Error(err, "failed to list HTTPRoutes", "namespace", pool.Namespace)
				return false
			}
			// Iterate over each HTTPRoute to see if any rule references this pool and has a matching controllerName.
			for _, route := range routes.Items {
				for _, rule := range route.Spec.Rules {
					for _, ref := range rule.BackendRefs {
						if ref.Kind != nil && *ref.Kind == wellknown.InferencePoolKind && string(ref.Name) == pool.Name {
							for _, p := range route.Status.Parents {
								if p.ControllerName == apiv1.GatewayController(poolCfg.ControllerName) {
									return true
								}
							}
						}
					}
				}
			}
			return false
		}
	}

	// TODO [danehans]: Make GatewayConfig optional since Gateway and InferencePool are independent controllers.
	controllerBuilder := &controllerBuilder{
		cfg:     *gwCfg,
		poolCfg: poolCfg,
		reconciler: &controllerReconciler{
			cli:    poolCfg.Mgr.GetClient(),
			scheme: poolCfg.Mgr.GetScheme(),
		},
	}

	return run(ctx, controllerBuilder.watchInferencePool)
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
	cfg        GatewayConfig
	poolCfg    *InferencePoolConfig
	reconciler *controllerReconciler
}

func (c *controllerBuilder) addGwParamsIndexes(ctx context.Context) error {
	return c.cfg.Mgr.GetFieldIndexer().IndexField(ctx, &apiv1.Gateway{}, GatewayParamsField, gatewayToParams)
}

// gatewayToParams is an IndexerFunc that gets a GatewayParameters name from a Gateway
func gatewayToParams(obj client.Object) []string {
	gw, ok := obj.(*apiv1.Gateway)
	if !ok {
		panic(fmt.Sprintf("wrong type %T provided to indexer. expected Gateway", obj))
	}
	gwpName := gw.GetAnnotations()[wellknown.GatewayParametersAnnotationName]
	if gwpName != "" {
		return []string{gwpName}
	}
	return []string{}
}

func (c *controllerBuilder) watchGw(ctx context.Context) error {
	// setup a deployer
	log := log.FromContext(ctx)

	log.Info("creating gateway deployer", "ctrlname", c.cfg.ControllerName, "server", c.cfg.ControlPlane.XdsHost, "port", c.cfg.ControlPlane.XdsPort)
	d, err := deployer.NewDeployer(c.cfg.Mgr.GetClient(), &deployer.Inputs{
		ControllerName:          c.cfg.ControllerName,
		Dev:                     c.cfg.Dev,
		IstioIntegrationEnabled: c.cfg.IstioIntegrationEnabled,
		ControlPlane:            c.cfg.ControlPlane,
		Aws:                     c.cfg.Aws,
	})
	if err != nil {
		return err
	}

	gvks, err := d.GetGvksToWatch(ctx)
	if err != nil {
		return err
	}

	buildr := ctrl.NewControllerManagedBy(c.cfg.Mgr).
		// Don't use WithEventFilter here as it also filters events for Owned objects.
		For(&apiv1.Gateway{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(object client.Object) bool {
			// We only care about Gateways that use our GatewayClass
			if gw, ok := object.(*apiv1.Gateway); ok {
				return c.cfg.OurGateway(gw)
			}
			return false
		}),
			predicate.Or(
				predicate.AnnotationChangedPredicate{},
				predicate.GenerationChangedPredicate{},
			),
		))

	// watch for changes in GatewayParameters
	cli := c.cfg.Mgr.GetClient()
	buildr.Watches(&v1alpha1.GatewayParameters{}, handler.EnqueueRequestsFromMapFunc(
		func(ctx context.Context, obj client.Object) []reconcile.Request {
			gwpName := obj.GetName()
			gwpNamespace := obj.GetNamespace()
			// look up the Gateways that are using this GatewayParameters object
			var gwList apiv1.GatewayList
			err := cli.List(ctx, &gwList, client.InNamespace(gwpNamespace), client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(GatewayParamsField, gwpName)})
			if err != nil {
				log.Error(err, "could not list Gateways using GatewayParameters", "gwpNamespace", gwpNamespace, "gwpName", gwpName)
				return []reconcile.Request{}
			}
			// reconcile each Gateway that is using this GatewayParameters object
			var reqs []reconcile.Request
			for _, gw := range gwList.Items {
				reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: gw.Namespace, Name: gw.Name}})
			}
			return reqs
		}))

	for _, gvk := range gvks {
		obj, err := c.cfg.Mgr.GetScheme().New(gvk)
		if err != nil {
			return err
		}
		clientObj, ok := obj.(client.Object)
		if !ok {
			return fmt.Errorf("object %T is not a client.Object", obj)
		}
		log.Info("watching gvk as gateway child", "gvk", gvk)
		// unless it's a service, we don't care about the status
		var opts []builder.OwnsOption
		if shouldIgnoreStatusChild(gvk) {
			opts = append(opts, builder.WithPredicates(predicate.GenerationChangedPredicate{}))
		}
		buildr.Owns(clientObj, opts...)
	}
	gwReconciler := &gatewayReconciler{
		cli:           c.cfg.Mgr.GetClient(),
		scheme:        c.cfg.Mgr.GetScheme(),
		autoProvision: c.cfg.AutoProvision,
		deployer:      d,
	}
	err = buildr.Complete(gwReconciler)
	if err != nil {
		return err
	}
	return nil
}

// watchInferencePool adds a watch on InferencePool objects (filtered by OurPool)
// as well as on HTTPRoute objects to trigger reconciliation for referenced pools.
func (c *controllerBuilder) watchInferencePool(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("creating inference extension deployer", "controller", c.cfg.ControllerName)

	d, err := deployer.NewDeployer(c.cfg.Mgr.GetClient(), &deployer.Inputs{
		ControllerName:     c.cfg.ControllerName,
		InferenceExtension: c.poolCfg.InferenceExt,
	})
	if err != nil {
		return err
	}

	buildr := ctrl.NewControllerManagedBy(c.cfg.Mgr).
		// For InferencePool objects that satisfy our OurPool predicate.
		For(&infextv1a1.InferencePool{}, builder.WithPredicates(
			predicate.NewPredicateFuncs(func(object client.Object) bool {
				if pool, ok := object.(*infextv1a1.InferencePool); ok {
					return c.poolCfg.OurPool(pool)
				}
				return false
			}),
			predicate.Or(
				predicate.AnnotationChangedPredicate{},
				predicate.GenerationChangedPredicate{},
			),
		)).
		// Watch HTTPRoute objects so that changes there trigger a reconcile for referenced pools.
		Watches(&apiv1.HTTPRoute{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			var reqs []reconcile.Request
			route, ok := obj.(*apiv1.HTTPRoute)
			if !ok {
				return nil
			}
			// For every backend ref in every rule of the route...
			for _, rule := range route.Spec.Rules {
				for _, ref := range rule.BackendRefs {
					if ref.Kind != nil && *ref.Kind == wellknown.InferencePoolKind {
						reqs = append(reqs, reconcile.Request{
							NamespacedName: client.ObjectKey{
								Namespace: route.Namespace,
								Name:      string(ref.Name),
							},
						})
					}
				}
			}
			return reqs
		}))

	// Watch child objects, e.g. Deployments, created by the inference pool deployer.
	gvks, err := d.GetGvksToWatch(ctx)
	if err != nil {
		return err
	}
	for _, gvk := range gvks {
		obj, err := c.cfg.Mgr.GetScheme().New(gvk)
		if err != nil {
			return err
		}
		clientObj, ok := obj.(client.Object)
		if !ok {
			return fmt.Errorf("object %T is not a client.Object", obj)
		}
		log.Info("watching gvk as inferencepool child", "gvk", gvk)
		var opts []builder.OwnsOption
		if shouldIgnoreStatusChild(gvk) {
			opts = append(opts, builder.WithPredicates(predicate.GenerationChangedPredicate{}))
		}
		buildr.Owns(clientObj, opts...)
	}

	r := &inferencePoolReconciler{
		cli:      c.cfg.Mgr.GetClient(),
		scheme:   c.cfg.Mgr.GetScheme(),
		deployer: d,
	}
	if err := buildr.Complete(r); err != nil {
		return err
	}

	return nil
}

func shouldIgnoreStatusChild(gvk schema.GroupVersionKind) bool {
	// avoid triggering on pod changes that update deployment status
	return gvk.Kind == "Deployment"
}

func (c *controllerBuilder) watchGwClass(_ context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			// we only care about GatewayClasses that use our controller name
			if gwClass, ok := object.(*apiv1.GatewayClass); ok {
				return gwClass.Spec.ControllerName == apiv1.GatewayController(c.cfg.ControllerName)
			}
			return false
		})).
		For(&apiv1.GatewayClass{}).
		Complete(reconcile.Func(c.reconciler.ReconcileGatewayClasses))
}

type controllerReconciler struct {
	cli    client.Client
	scheme *runtime.Scheme
}

func (r *controllerReconciler) ReconcileGatewayClasses(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("gwclass", req.NamespacedName)

	gwclass := &apiv1.GatewayClass{}
	if err := r.cli.Get(ctx, req.NamespacedName, gwclass); err != nil {
		// NOTE: if this reconciliation is a result of a DELETE event, this err will be a NotFound,
		// therefore we will return a nil error here and thus skip any additional reconciliation below.
		// At the time of writing this comment, the retrieved GWClass object is only used to update the status,
		// so it should be fine to return here, because there's no status update needed on a deleted resource.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("reconciling gateway class")

	// mark it as accepted:
	acceptedCondition := metav1.Condition{
		Type:               string(apiv1.GatewayClassConditionStatusAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(apiv1.GatewayClassReasonAccepted),
		ObservedGeneration: gwclass.Generation,
		// no need to set LastTransitionTime, it will be set automatically by SetStatusCondition
	}
	meta.SetStatusCondition(&gwclass.Status.Conditions, acceptedCondition)

	// TODO: This should actually check the version of the CRDs in the cluster to be 100% sure
	supportedVersionCondition := metav1.Condition{
		Type:               string(apiv1.GatewayClassConditionStatusSupportedVersion),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gwclass.Generation,
		Reason:             string(apiv1.GatewayClassReasonSupportedVersion),
	}
	meta.SetStatusCondition(&gwclass.Status.Conditions, supportedVersionCondition)

	if err := r.cli.Status().Update(ctx, gwclass); err != nil {
		return ctrl.Result{}, err
	}
	log.Info("updated gateway class status")

	return ctrl.Result{}, nil
}
