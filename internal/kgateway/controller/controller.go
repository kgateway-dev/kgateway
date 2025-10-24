package controller

import (
	"context"
	"fmt"

	"istio.io/istio/pkg/kube/kubetypes"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"

	internaldeployer "github.com/kgateway-dev/kgateway/v2/internal/kgateway/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
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
	Client apiclient.Client
	Mgr    manager.Manager
	// Dev enables development mode for the controller.
	Dev bool
	// ControllerName is the name of the Envoy controller. Any GatewayClass objects
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

type HelmValuesGeneratorOverrideFunc func(inputs *deployer.Inputs) deployer.HelmValuesGenerator

func NewBaseGatewayController(
	ctx context.Context,
	cfg GatewayConfig,
	classInfos map[string]*deployer.GatewayClassInfo,
	helmValuesGeneratorOverride HelmValuesGeneratorOverrideFunc,
	gatewayControllerExtension pluginsdk.GatewayControllerExtension,
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
		gatewayControllerExtension:  gatewayControllerExtension,
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
	helmValuesGeneratorOverride func(inputs *deployer.Inputs) deployer.HelmValuesGenerator
	gatewayControllerExtension  pluginsdk.GatewayControllerExtension
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
	logger.Info("creating gateway deployer",
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

	gwParams := internaldeployer.NewGatewayParameters(c.cfg.Client, inputs)
	if c.helmValuesGeneratorOverride != nil {
		gwParams.WithHelmValuesGeneratorOverride(c.helmValuesGeneratorOverride(inputs))
	}

	d, err := internaldeployer.NewGatewayDeployer(
		c.cfg.ControllerName,
		c.cfg.AgwControllerName,
		c.cfg.AgentgatewayClassName,
		c.cfg.Mgr.GetScheme(),
		c.cfg.Client,
		gwParams,
	)
	if err != nil {
		return err
	}

	if err := c.cfg.Mgr.Add(NewGatewayReconciler(ctx, c.cfg, d, gwParams, c.gatewayControllerExtension)); err != nil {
		return err
	}

	return nil
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
	customEvents chan event.TypedGenericEvent[ir.GatewayForDeployer]
	metricsName  string
	classInfos   map[string]*deployer.GatewayClassInfo
}

func (r *controllerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, rErr error) {
	log := log.FromContext(ctx).WithValues("gwc", req.NamespacedName)
	log.Info("reconciling gateway class")
	defer log.Info("finished reconciling gateway class")

	finishMetrics := collectReconciliationMetrics(r.metricsName, req.NamespacedName)
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
