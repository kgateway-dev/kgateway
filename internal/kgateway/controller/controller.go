package controller

import (
	"context"
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"

	internaldeployer "github.com/kgateway-dev/kgateway/v2/internal/kgateway/deployer"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

const (
	GatewayClassField = "spec.gatewayClassName"
	// GatewayParamsField is the field name used for indexing Gateway objects.
	GatewayParamsField = "gateway-params"
	// InferencePoolField is the field name used for indexing HTTPRoute objects.
	InferencePoolField = "inferencepool-index"
)

// TODO [danehans]: Refactor so controller config is organized into shared and Gateway/InferencePool-specific controllers.
type GatewayConfig struct {
	Mgr manager.Manager
	// Dev enables development mode for the controller.
	Dev bool
	// ControllerName is the name of the controller. Any GatewayClass objects
	// managed by this controller must have this name as their ControllerName.
	ControllerName string
	// AgwControllerName is the name of the agentgateway controller. Any GatewayClass objects
	// managed by this controller must have this name as their ControllerName.
	AgwControllerName string
	// AutoProvision enables auto-provisioning of GatewayClasses.
	AutoProvision bool
	// ControlPlane sets the default control plane information the deployer will use.
	ControlPlane deployer.ControlPlaneInfo
	// IstioAutoMtlsEnabled enables istio auto mtls mode for the controller,
	// resulting in the deployer to enable istio and sds sidecars on the deployed proxies.
	IstioAutoMtlsEnabled bool
	// ImageInfo sets the default image information the deployer will use.
	ImageInfo *deployer.ImageInfo
	// DiscoveryNamespaceFilter filters namespaced objects based on the discovery namespace filter.
	DiscoveryNamespaceFilter kubetypes.DynamicObjectFilter
	// CommonCollections used to fetch ir.Gateways for the deployer to generate the ports for the proxy service
	CommonCollections *collections.CommonCollections
	// GatewayClassName is the configured gateway class name.
	GatewayClassName string
	// WaypointGatewayClassName is the configured waypoint gateway class name.
	WaypointGatewayClassName string
	// AgentgatewayClassName is the configured agent gateway class name.
	AgentgatewayClassName string
	// Additional GatewayClass definitions to support extending to other well-known gateway classes
	AdditionalGatewayClasses map[string]*deployer.GatewayClassInfo
}

type ExtraGatewayParametersFunc func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters

func NewBaseGatewayController(ctx context.Context, cfg GatewayConfig, extraGatewayParameters ExtraGatewayParametersFunc) error {
	log := log.FromContext(ctx)
	log.V(5).Info("starting gateway controller", "controllerName", cfg.ControllerName)

	controllerBuilder := &controllerBuilder{
		cfg: cfg,
		reconciler: &controllerReconciler{
			cli:          cfg.Mgr.GetClient(),
			scheme:       cfg.Mgr.GetScheme(),
			customEvents: make(chan event.TypedGenericEvent[ir.Gateway], 1024),
			metricsName:  "gatewayclass",
		},
		extraGatewayParameters: extraGatewayParameters,
	}

	return run(
		ctx,
		controllerBuilder.watchGwClass,
		controllerBuilder.watchGw,
		controllerBuilder.addIndexes,
	)
}

type InferencePoolConfig struct {
	Mgr            manager.Manager
	ControllerName string
	InferenceExt   *deployer.InferenceExtInfo
}

func NewBaseInferencePoolController(ctx context.Context,
	poolCfg *InferencePoolConfig,
	gwCfg *GatewayConfig,
	extraGatewayParameters func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters) error {
	log := log.FromContext(ctx)
	log.V(5).Info("starting inferencepool controller", "controllerName", poolCfg.ControllerName)

	// TODO [danehans]: Make GatewayConfig optional since Gateway and InferencePool are independent controllers.
	controllerBuilder := &controllerBuilder{
		cfg:     *gwCfg,
		poolCfg: poolCfg,
		reconciler: &controllerReconciler{
			cli:          poolCfg.Mgr.GetClient(),
			scheme:       poolCfg.Mgr.GetScheme(),
			customEvents: make(chan event.TypedGenericEvent[ir.Gateway], 1024),
			metricsName:  "gatewayclass-inferencepool",
		},
		extraGatewayParameters: extraGatewayParameters,
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
	cfg                    GatewayConfig
	poolCfg                *InferencePoolConfig
	reconciler             *controllerReconciler
	extraGatewayParameters func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters
}

func (c *controllerBuilder) addIndexes(ctx context.Context) error {
	if err := c.cfg.Mgr.GetFieldIndexer().IndexField(ctx, &apiv1.Gateway{}, GatewayParamsField, gatewayToParams); err != nil {
		return err
	}
	if err := c.cfg.Mgr.GetFieldIndexer().IndexField(ctx, &apiv1.Gateway{}, GatewayClassField, gatewayToClass); err != nil {
		return err
	}
	return nil
}

// gatewayToParams is an IndexerFunc that gets a GatewayParameters name from a Gateway.
// It checks the Gateway's spec.infrastructure.parametersRef, or returns an empty
// slice when it's not set.
func gatewayToParams(obj client.Object) []string {
	gw, ok := obj.(*apiv1.Gateway)
	if !ok {
		panic(fmt.Sprintf("wrong type %T provided to indexer. expected Gateway", obj))
	}
	infrastructureRef := gw.Spec.Infrastructure
	if infrastructureRef != nil && infrastructureRef.ParametersRef != nil {
		return []string{infrastructureRef.ParametersRef.Name}
	}
	return []string{}
}

// gatewayToClass is an IndexerFunc that lists a Gateways that use a given className
func gatewayToClass(obj client.Object) []string {
	gw, ok := obj.(*apiv1.Gateway)
	if !ok {
		panic(fmt.Sprintf("wrong type %T provided to indexer. expected Gateway", obj))
	}
	return []string{string(gw.Spec.GatewayClassName)}
}

func (c *controllerBuilder) watchGw(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("creating gateway deployer", "ctrlname", c.cfg.ControllerName, "agwctrlname",
		c.cfg.AgwControllerName, "server", c.cfg.ControlPlane.XdsHost, "port", c.cfg.ControlPlane.XdsPort, "agwport", c.cfg.ControlPlane.AgwXdsPort)

	inputs := &deployer.Inputs{
		Dev:                      c.cfg.Dev,
		IstioAutoMtlsEnabled:     c.cfg.IstioAutoMtlsEnabled,
		ControlPlane:             c.cfg.ControlPlane,
		ImageInfo:                c.cfg.ImageInfo,
		CommonCollections:        c.cfg.CommonCollections,
		GatewayClassName:         c.cfg.GatewayClassName,
		WaypointGatewayClassName: c.cfg.WaypointGatewayClassName,
		AgentgatewayClassName:    c.cfg.AgentgatewayClassName,
	}

	gwParams := internaldeployer.NewGatewayParameters(c.cfg.Mgr.GetClient(), inputs)
	if c.extraGatewayParameters != nil {
		gwParams.WithExtraGatewayParameters(c.extraGatewayParameters(c.cfg.Mgr.GetClient(), inputs)...)
	}

	discoveryNamespaceFilterPredicate := predicate.NewPredicateFuncs(func(o client.Object) bool {
		filter := c.cfg.DiscoveryNamespaceFilter.Filter(o)
		return filter
	})
	buildr := ctrl.NewControllerManagedBy(c.cfg.Mgr).
		WithEventFilter(discoveryNamespaceFilterPredicate).
		// Don't use WithEventFilter here as it also filters events for Owned objects.
		For(&apiv1.Gateway{}, builder.WithPredicates(
			// TODO(stevenctl) investigate perf implications of filtering in Reconcile
			// the tricky part is we want to check a relationship of gateway -> gatewayclass -> controller name
			predicate.Or(
				predicate.AnnotationChangedPredicate{},
				predicate.GenerationChangedPredicate{},
			),
		),
		)

	// watch for changes in GatewayParameters and enqueue Gateways that use them
	cli := c.cfg.Mgr.GetClient()
	for _, gp := range gwParams.AllKnownGatewayParameters() {
		buildr.Watches(gp, handler.EnqueueRequestsFromMapFunc(
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
				// requeue each Gateway that is using this GatewayParameters object
				reqs := make([]reconcile.Request, 0, len(gwList.Items))
				for _, gw := range gwList.Items {
					reqs = append(reqs, reconcile.Request{
						NamespacedName: client.ObjectKeyFromObject(&gw),
					})
				}
				return reqs
			}),
			builder.WithPredicates(discoveryNamespaceFilterPredicate),
		)
	}

	// watch for gatewayclasses managed by our controller and enqueue related gateways
	buildr.Watches(
		&apiv1.GatewayClass{},
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			gc, ok := obj.(*apiv1.GatewayClass)
			if !ok {
				return nil
			}
			var gwList apiv1.GatewayList
			if err := c.cfg.Mgr.GetClient().List(
				ctx,
				&gwList,
				client.MatchingFields{GatewayClassField: gc.Name},
			); err != nil {
				log.Error(err, "failed listing GatewayClasses in predicate")
				return nil
			}
			reqs := make([]reconcile.Request, 0, len(gwList.Items))
			for _, gw := range gwList.Items {
				if c.cfg.DiscoveryNamespaceFilter.Filter(&gw) {
					reqs = append(reqs,
						reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&gw)},
					)
				}
			}
			return reqs
		}),
		builder.WithPredicates(
			predicate.NewPredicateFuncs(func(o client.Object) bool {
				gc, ok := o.(*apiv1.GatewayClass)
				// filter for both kgateway and agentgateway controller names
				return ok && (gc.Spec.ControllerName == apiv1.GatewayController(c.cfg.ControllerName) ||
					gc.Spec.ControllerName == apiv1.GatewayController(c.cfg.AgwControllerName))
			}),
			predicate.GenerationChangedPredicate{},
		),
	)

	// Trigger an event when the gateway changes. This can even be a change in listener sets attached to the gateway
	c.cfg.CommonCollections.GatewayIndex.Gateways.Register(func(o krt.Event[ir.Gateway]) {
		gw := o.Latest()
		c.reconciler.customEvents <- event.TypedGenericEvent[ir.Gateway]{
			Object: gw,
		}
	})
	buildr.WatchesRawSource(
		// Add channel source for custom events
		source.Channel(
			c.reconciler.customEvents,
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, obj ir.Gateway) []reconcile.Request {
				// Convert the generic event to a reconcile request
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name},
					},
				}
			}),
		),
	)

	d, err := internaldeployer.NewGatewayDeployer(
		c.cfg.ControllerName,
		c.cfg.AgwControllerName,
		c.cfg.AgentgatewayClassName,
		c.cfg.Mgr.GetClient(),
		gwParams,
	)
	if err != nil {
		return err
	}

	gvks, err := internaldeployer.GatewayGVKsToWatch(ctx, d)
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
		log.Info("watching gvk as gateway child", "gvk", gvk)
		// unless it's a service, we don't care about the status
		var opts []builder.OwnsOption
		if shouldIgnoreStatusChild(gvk) {
			opts = append(opts, builder.WithPredicates(predicate.GenerationChangedPredicate{}))
		}
		buildr.Owns(clientObj, opts...)
	}

	// The controller should only run on the leader as the gatewayReconciler manages reconciliation.
	// It deploys and manages the relevant resources (deployment, service, etc.) and should run only on the leader.
	// This is the default behaviour. Ref: https://github.com/kubernetes-sigs/controller-runtime/blob/682465344b9b74efad4657016668e62438000541/pkg/internal/controller/controller.go#L223
	// but calling it out explicitly here as the gatewayReconciler is not directly added
	// as a runnable to the manager and can not be static typed as a manager.LeaderElectionRunnable
	// Translation is managed by the proxySyncer and runs on all pods (leader and follower)
	buildr.WithOptions(controller.TypedOptions[reconcile.Request]{
		NeedLeaderElection: ptr.To(true),
	})
	return buildr.Complete(NewGatewayReconciler(ctx, c.cfg, d))
}

func (c *controllerBuilder) addHTTPRouteIndexes(ctx context.Context) error {
	return c.cfg.Mgr.GetFieldIndexer().IndexField(ctx, new(apiv1.HTTPRoute), InferencePoolField, httpRouteInferencePoolIndex)
}

func httpRouteInferencePoolIndex(obj client.Object) []string {
	route, ok := obj.(*apiv1.HTTPRoute)
	if !ok {
		// Should never happen, but return empty slice in case of unexpected type.
		return nil
	}

	var poolNames []string
	for _, rule := range route.Spec.Rules {
		for _, ref := range rule.BackendRefs {
			if ref.Kind != nil && *ref.Kind == wellknown.InferencePoolKind {
				poolNames = append(poolNames, string(ref.Name))
			}
		}
	}
	return poolNames
}

// watchInferencePool adds a watch on InferencePool and HTTPRoute objects (that reference an InferencePool)
// to trigger reconciliation.
func (c *controllerBuilder) watchInferencePool(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("creating inference extension deployer", "controller", c.cfg.ControllerName)

	// Register the HTTPRoute index.
	if err := c.addHTTPRouteIndexes(ctx); err != nil {
		return fmt.Errorf("failed to register HTTPRoute index: %w", err)
	}

	discoveryNamespaceFilterPredicate := predicate.NewPredicateFuncs(func(o client.Object) bool {
		return c.cfg.DiscoveryNamespaceFilter.Filter(o)
	})

	buildr := ctrl.NewControllerManagedBy(c.cfg.Mgr).
		WithEventFilter(discoveryNamespaceFilterPredicate).
		For(&inf.InferencePool{}, builder.WithPredicates(
			predicate.Or(
				predicate.AnnotationChangedPredicate{},
				predicate.GenerationChangedPredicate{},
			),
		)).
		// Watch HTTPRoute objects so that changes there trigger a reconcile for referenced InferencePools.
		Watches(&apiv1.HTTPRoute{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			route, ok := obj.(*apiv1.HTTPRoute)
			if !ok {
				return nil
			}

			// Use the index function to get the inference pool names.
			poolNames := httpRouteInferencePoolIndex(route)
			if len(poolNames) == 0 {
				return nil
			}

			hasOurGateway := false
			for _, pStatus := range route.Status.Parents {
				if pStatus.ControllerName == apiv1.GatewayController(c.cfg.ControllerName) {
					hasOurGateway = true
					break
				}
			}
			if !hasOurGateway {
				// If no parentRef references one of our Gateways, skip it.
				return nil
			}

			// The HTTPRoute references an InferencePool and one of our Gateways.
			// Enqueue each referenced InferencePool for reconciliation.
			var reqs []reconcile.Request
			for _, poolName := range poolNames {
				reqs = append(reqs, reconcile.Request{
					NamespacedName: client.ObjectKey{
						Namespace: route.Namespace,
						Name:      poolName,
					},
				})
			}
			return reqs
		}),
			builder.WithPredicates(discoveryNamespaceFilterPredicate),
		)

	// If enabled, create a deployer using the controllerBuilder as inputs.
	if c.poolCfg.InferenceExt != nil {
		d, err := internaldeployer.NewInferencePoolDeployer(c.cfg.ControllerName, c.cfg.AgwControllerName, c.cfg.AgentgatewayClassName, c.cfg.Mgr.GetClient())
		if err != nil {
			return err
		}
		// Watch child objects, e.g. Deployments, created by the inference pool deployer.
		gvks, err := internaldeployer.InferencePoolGVKsToWatch(ctx, d)
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

		// The controller should only run on the leader as the inferencePoolReconciler manages reconciliation.
		// It deploys and manages the relevant resources (deployment, service, etc.) and should run only on the leader.
		// This is the default behaviour. Ref: https://github.com/kubernetes-sigs/controller-runtime/blob/682465344b9b74efad4657016668e62438000541/pkg/internal/controller/controller.go#L223
		// but calling it out explicitly here as the inferencePoolReconciler is not directly added
		// as a runnable to the manager and can not be static typed as a manager.LeaderElectionRunnable
		// Translation is managed by the proxySyncer and runs on all pods (leader and follower)
		buildr.WithOptions(controller.TypedOptions[reconcile.Request]{
			NeedLeaderElection: ptr.To(true),
		})
		if err := buildr.Complete(r); err != nil {
			return err
		}
	}

	return nil
}

func shouldIgnoreStatusChild(gvk schema.GroupVersionKind) bool {
	// avoid triggering on pod changes that update deployment status
	return gvk.Kind == "Deployment"
}

func (c *controllerBuilder) watchGwClass(_ context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		For(&apiv1.GatewayClass{}, builder.WithPredicates(predicate.Funcs{
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			UpdateFunc:  func(e event.UpdateEvent) bool { return true },
			GenericFunc: func(e event.GenericEvent) bool { return false },
		})).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			// we only care about GatewayClasses that use our controller name
			gwClass, ok := object.(*apiv1.GatewayClass)
			return ok && (gwClass.Spec.ControllerName == apiv1.GatewayController(c.cfg.ControllerName) ||
				gwClass.Spec.ControllerName == apiv1.GatewayController(c.cfg.AgwControllerName))
		})).
		Complete(c.reconciler)
}

type controllerReconciler struct {
	cli          client.Client
	scheme       *runtime.Scheme
	customEvents chan event.TypedGenericEvent[ir.Gateway]
	metricsName  string
}

func (r *controllerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, rErr error) {
	log := log.FromContext(ctx).WithValues("gwclass", req.NamespacedName)

	finishMetrics := collectReconciliationMetrics(r.metricsName, req)
	defer func() {
		finishMetrics(rErr)
	}()

	gwclass := &apiv1.GatewayClass{}
	if err := r.cli.Get(ctx, req.NamespacedName, gwclass); err != nil {
		// NOTE: if this reconciliation is a result of a DELETE event, this err will be a NotFound,
		// therefore we will return a nil error here and thus skip any additional reconciliation below.
		// At the time of writing this comment, the retrieved GWClass object is only used to update the status,
		// so it should be fine to return here, because there's no status update needed on a deleted resource.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("reconciling gateway class")

	meta.SetStatusCondition(&gwclass.Status.Conditions, metav1.Condition{
		Type:               string(apiv1.GatewayClassConditionStatusAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(apiv1.GatewayClassReasonAccepted),
		ObservedGeneration: gwclass.Generation,
		Message:            "GatewayClass accepted by kgateway controller",
	})

	if err := r.cli.Status().Update(ctx, gwclass); err != nil {
		return ctrl.Result{}, err
	}
	log.Info("updated gateway class status")

	return ctrl.Result{}, nil
}
