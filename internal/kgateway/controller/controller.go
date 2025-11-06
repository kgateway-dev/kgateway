package controller

import (
	"context"
	"crypto/tls"
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
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"

	internaldeployer "github.com/kgateway-dev/kgateway/v2/internal/kgateway/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

const (
	GatewayClassField = "spec.gatewayClassName"
	// GatewayParamsField is the field name used for indexing Gateway objects.
	GatewayParamsField = "gateway-params"
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
	// CertWatcher is the shared certificate watcher for xDS TLS
	CertWatcher *certwatcher.CertWatcher
}

type HelmValuesGeneratorOverrideFunc func(cli client.Client, inputs *deployer.Inputs) deployer.HelmValuesGenerator

func NewBaseGatewayController(
	ctx context.Context,
	cfg GatewayConfig,
	classInfos map[string]*deployer.GatewayClassInfo,
	helmValuesGeneratorOverride HelmValuesGeneratorOverrideFunc,
	extraGatewayParameters []client.Object,
) error {
	log := log.FromContext(ctx)
	log.V(5).Info("starting gateway controller", "controllerName", cfg.ControllerName)

	controllerBuilder := &controllerBuilder{
		cfg: cfg,
		reconciler: &controllerReconciler{
			cli:          cfg.Mgr.GetClient(),
			scheme:       cfg.Mgr.GetScheme(),
			customEvents: make(chan event.TypedGenericEvent[ir.GatewayForDeployer], 1024),
			metricsName:  "gatewayclass",
			classInfos:   classInfos,
		},
		helmValuesGeneratorOverride: helmValuesGeneratorOverride,
		extraGatewayParameters:      extraGatewayParameters,
	}

	return run(
		ctx,
		controllerBuilder.watchGwClass,
		controllerBuilder.watchGw,
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
	cfg                         GatewayConfig
	reconciler                  *controllerReconciler
	helmValuesGeneratorOverride func(cli client.Client, inputs *deployer.Inputs) deployer.HelmValuesGenerator
	extraGatewayParameters      []client.Object
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
	log.Info("creating gateway deployer",
		"ctrlname", c.cfg.ControllerName, "agwctrlname", c.cfg.AgwControllerName,
		"server", c.cfg.ControlPlane.XdsHost, "port", c.cfg.ControlPlane.XdsPort,
		"agwport", c.cfg.ControlPlane.AgwXdsPort, "tls", c.cfg.ControlPlane.XdsTLS,
	)

	inputs := &deployer.Inputs{
		Dev:                        c.cfg.Dev,
		IstioAutoMtlsEnabled:       c.cfg.IstioAutoMtlsEnabled,
		ControlPlane:               c.cfg.ControlPlane,
		ImageInfo:                  c.cfg.ImageInfo,
		CommonCollections:          c.cfg.CommonCollections,
		GatewayClassName:           c.cfg.GatewayClassName,
		WaypointGatewayClassName:   c.cfg.WaypointGatewayClassName,
		AgentgatewayClassName:      c.cfg.AgentgatewayClassName,
		AgentgatewayControllerName: c.cfg.AgwControllerName,
	}

	gwParams := internaldeployer.NewGatewayParameters(c.cfg.Mgr.GetClient(), inputs)
	if c.helmValuesGeneratorOverride != nil {
		gwParams.WithHelmValuesGeneratorOverride(c.helmValuesGeneratorOverride(c.cfg.Mgr.GetClient(), inputs))
	}
	if len(c.extraGatewayParameters) > 0 {
		gwParams.WithExtraGatewayParameters(c.extraGatewayParameters...)
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

				reqs := []reconcile.Request{}

				// 1. Look up Gateways directly using this GatewayParameters object (via spec.infrastructure.parametersRef)
				var gwList apiv1.GatewayList
				err := cli.List(ctx, &gwList, client.InNamespace(gwpNamespace), client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(GatewayParamsField, gwpName)})
				if err != nil {
					log.Error(err, "could not list Gateways using GatewayParameters", "gwpNamespace", gwpNamespace, "gwpName", gwpName)
				} else {
					for _, gw := range gwList.Items {
						reqs = append(reqs, reconcile.Request{
							NamespacedName: client.ObjectKeyFromObject(&gw),
						})
					}
				}

				// 2. Look up GatewayClasses using this GatewayParameters object (via spec.parametersRef)
				var gcList apiv1.GatewayClassList
				err = cli.List(ctx, &gcList)
				if err != nil {
					log.Error(err, "could not list GatewayClasses")
					return reqs
				}

				// For each GatewayClass that references this parameter, find all Gateways using that class
				for _, gc := range gcList.Items {
					// Only process GatewayClasses managed by our controllers
					if gc.Spec.ControllerName != apiv1.GatewayController(c.cfg.ControllerName) &&
						gc.Spec.ControllerName != apiv1.GatewayController(c.cfg.AgwControllerName) {
						continue
					}
					if gc.Spec.ParametersRef != nil &&
						gc.Spec.ParametersRef.Name == gwpName &&
						gc.Spec.ParametersRef.Namespace != nil && string(*gc.Spec.ParametersRef.Namespace) == gwpNamespace {
						// This GatewayClass references our GatewayParameters, find all Gateways using this class
						var classGwList apiv1.GatewayList
						err := cli.List(ctx, &classGwList, client.MatchingFields{GatewayClassField: gc.Name})
						if err != nil {
							log.Error(err, "could not list Gateways for GatewayClass", "gatewayClassName", gc.Name)
							continue
						}
						for _, gw := range classGwList.Items {
							if c.cfg.DiscoveryNamespaceFilter.Filter(&gw) {
								reqs = append(reqs, reconcile.Request{
									NamespacedName: client.ObjectKeyFromObject(&gw),
								})
							}
						}
					}
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
	c.cfg.CommonCollections.GatewayIndex.GatewaysForDeployer.Register(func(o krt.Event[ir.GatewayForDeployer]) {
		gw := o.Latest()
		c.reconciler.customEvents <- event.TypedGenericEvent[ir.GatewayForDeployer]{
			Object: gw,
		}
	})
	buildr.WatchesRawSource(
		// Add channel source for custom events
		source.Channel(
			c.reconciler.customEvents,
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, obj ir.GatewayForDeployer) []reconcile.Request {
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

	// Watch for xDS TLS certificate changes to update proxy CA certificates. Kick reconciliation for
	// all Gateways managed by our controllers when the xDS TLS certificate changes.
	c.setupTLSCertificateWatch(ctx, buildr)

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

// setupTLSCertificateWatch configures a watch for xDS TLS certificate changes.
// When certificates are rotated, all Gateways managed by this controller will be reconciled
// to update the proxy CA certificates.
func (c *controllerBuilder) setupTLSCertificateWatch(ctx context.Context, buildr *builder.Builder) {
	if c.cfg.CertWatcher == nil {
		return
	}

	log := log.FromContext(ctx)
	certChangeCh := make(chan event.GenericEvent, 1)
	// Register callback to send events when certificate changes
	c.cfg.CertWatcher.RegisterCallback(func(_ tls.Certificate) {
		log.Info("xDS TLS certificate changed, triggering Gateway reconciliation")
		select {
		case certChangeCh <- event.GenericEvent{}:
			log.V(1).Info("Sent certificate change event to Gateway controller")
		default:
			log.Info("Gateway controller channel full, skipping certificate change notification")
		}
	})
	// Watch the certificate change channel and reconcile affected Gateways
	buildr.WatchesRawSource(source.Channel(certChangeCh, handler.EnqueueRequestsFromMapFunc(
		func(ctx context.Context, obj client.Object) []reconcile.Request {
			var gwList apiv1.GatewayList
			if err := c.cfg.Mgr.GetClient().List(ctx, &gwList); err != nil {
				log.Error(err, "failed to list Gateways for certificate change")
				return nil
			}
			reqs := make([]reconcile.Request, 0, len(gwList.Items))
			for _, gw := range gwList.Items {
				var gwc apiv1.GatewayClass
				if err := c.cfg.Mgr.GetClient().Get(ctx, client.ObjectKey{Name: string(gw.Spec.GatewayClassName)}, &gwc); err != nil {
					log.Error(err, "failed to get GatewayClass for Gateway", "gateway", gw.Name)
					continue
				}
				if gwc.Spec.ControllerName == apiv1.GatewayController(c.cfg.ControllerName) ||
					gwc.Spec.ControllerName == apiv1.GatewayController(c.cfg.AgwControllerName) {
					reqs = append(reqs, reconcile.Request{
						NamespacedName: client.ObjectKeyFromObject(&gw),
					})
				}
			}
			return reqs
		}),
	))
}

type controllerReconciler struct {
	cli          client.Client
	scheme       *runtime.Scheme
	customEvents chan event.TypedGenericEvent[ir.GatewayForDeployer]
	metricsName  string
	classInfos   map[string]*deployer.GatewayClassInfo
}

func (r *controllerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, rErr error) {
	log := log.FromContext(ctx).WithValues("gwc", req.NamespacedName)
	log.Info("reconciling gateway class")
	defer log.Info("finished reconciling gateway class")

	finishMetrics := collectReconciliationMetrics(r.metricsName, req)
	defer func() {
		finishMetrics(rErr)
	}()

	gwc := &apiv1.GatewayClass{}
	if err := r.cli.Get(ctx, req.NamespacedName, gwc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	meta.SetStatusCondition(&gwc.Status.Conditions, metav1.Condition{
		Type:               string(apiv1.GatewayClassConditionStatusAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(apiv1.GatewayClassReasonAccepted),
		ObservedGeneration: gwc.GetGeneration(),
		Message:            reports.GatewayClassAcceptedMessage,
	})
	if i, ok := r.classInfos[gwc.GetName()]; ok {
		gwc.Status.SupportedFeatures = i.SupportedFeatures
	}

	if err := r.cli.Status().Update(ctx, gwc); err != nil {
		return ctrl.Result{}, err
	}
	log.Info("updated gateway class status")

	return ctrl.Result{}, nil
}
