package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	apiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/gateway-api/pkg/consts"

	gloov1 "github.com/solo-io/gloo/projects/controller/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/crds"
	"github.com/solo-io/gloo/projects/gateway2/deployer"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/query"
	httplisoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/httplisteneroptions/query"
	lisoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/listeneroptions/query"
	rtoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/routeoptions/query"
	vhoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/virtualhostoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
)

const (
	// field name used for indexing
	GatewayParamsField = "gateway-params"
)

type GatewayConfig struct {
	Mgr            manager.Manager
	GWClasses      sets.Set[string]
	Dev            bool
	ControllerName string
	AutoProvision  bool
	Kick           func(ctx context.Context)

	ControlPlane            deployer.ControlPlaneInfo
	IstioIntegrationEnabled bool
	Aws                     *deployer.AwsInfo

	Extensions extensions.K8sGatewayExtensions
	// CRDs is a map of supported Gateway API CRDs with the discovered annotations for each.
	CRDs *crds.CrdToAnnotation
}

func NewBaseGatewayController(ctx context.Context, cfg GatewayConfig) error {
	log := log.FromContext(ctx)
	log.V(5).Info("starting controller", "controllerName", cfg.ControllerName, "GatewayClasses", sets.List(cfg.GWClasses))

	controllerBuilder := &controllerBuilder{
		cfg: cfg,
		reconciler: &controllerReconciler{
			controllerName: cfg.ControllerName,
			cli:            cfg.Mgr.GetClient(),
			scheme:         cfg.Mgr.GetScheme(),
			kick:           cfg.Kick,
			crds:           cfg.CRDs,
		},
	}

	return run(ctx,
		controllerBuilder.watchCRDs,
		controllerBuilder.watchGwClass,
		controllerBuilder.watchGw,
		controllerBuilder.watchHttpRoute,
		controllerBuilder.watchTcpRoute,
		controllerBuilder.watchReferenceGrant,
		controllerBuilder.watchNamespaces,
		controllerBuilder.watchHttpListenerOptions,
		controllerBuilder.watchListenerOptions,
		controllerBuilder.watchRouteOptions,
		controllerBuilder.watchVirtualHostOptions,
		controllerBuilder.watchUpstreams,
		controllerBuilder.watchServices,
		controllerBuilder.watchEndpointSlices,
		controllerBuilder.watchPods,
		controllerBuilder.watchSecrets,
		controllerBuilder.addIndexes,
		controllerBuilder.addHttpLisOptIndexes,
		controllerBuilder.addLisOptIndexes,
		controllerBuilder.addRtOptIndexes,
		controllerBuilder.addVhOptIndexes,
		controllerBuilder.addGwParamsIndexes,
		controllerBuilder.watchDirectResponses,
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
	cfg GatewayConfig

	reconciler *controllerReconciler
}

func (c *controllerBuilder) addIndexes(ctx context.Context) error {
	var errs []error

	// Index for HTTPRoute
	if err := c.cfg.Mgr.GetFieldIndexer().IndexField(ctx, &apiv1.HTTPRoute{}, query.HttpRouteTargetField, query.IndexerByObjType); err != nil {
		errs = append(errs, err)
	}

	// Index for ReferenceGrant
	if err := c.cfg.Mgr.GetFieldIndexer().IndexField(ctx, &apiv1beta1.ReferenceGrant{}, query.ReferenceGrantFromField, query.IndexerByObjType); err != nil {
		errs = append(errs, err)
	}

	// Conditionally index for TCPRoute
	if c.cfg.CRDs != nil {
		if _, exists := (*c.cfg.CRDs)[crds.TCPRoute]; exists {
			if err := c.cfg.Mgr.GetFieldIndexer().IndexField(ctx, &apiv1a2.TCPRoute{}, query.TcpRouteTargetField, query.IndexerByObjType); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
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

// TODO: move to RtOpt plugin when breaking the logic to RouteOption-specific controller
func (c *controllerBuilder) addRtOptIndexes(ctx context.Context) error {
	return rtoptquery.IterateIndices(func(obj client.Object, field string, indexer client.IndexerFunc) error {
		return c.cfg.Mgr.GetFieldIndexer().IndexField(ctx, obj, field, indexer)
	})
}

// TODO: move to VhOpt plugin when breaking the logic to VirtualHostOption-specific controller
func (c *controllerBuilder) addVhOptIndexes(ctx context.Context) error {
	return vhoptquery.IterateIndices(func(obj client.Object, field string, indexer client.IndexerFunc) error {
		return c.cfg.Mgr.GetFieldIndexer().IndexField(ctx, obj, field, indexer)
	})
}

// TODO: move to LisOpt plugin when breaking the logic to ListenerOption-specific controller
func (c *controllerBuilder) addLisOptIndexes(ctx context.Context) error {
	return lisoptquery.IterateIndices(func(obj client.Object, field string, indexer client.IndexerFunc) error {
		return c.cfg.Mgr.GetFieldIndexer().IndexField(ctx, obj, field, indexer)
	})
}

// TODO: move to HttpLisOpt plugin when breaking the logic to HttpListenerOption-specific controller
func (c *controllerBuilder) addHttpLisOptIndexes(ctx context.Context) error {
	return httplisoptquery.IterateIndices(func(obj client.Object, field string, indexer client.IndexerFunc) error {
		return c.cfg.Mgr.GetFieldIndexer().IndexField(ctx, obj, field, indexer)
	})
}

func (c *controllerBuilder) watchGw(ctx context.Context) error {
	// setup a deployer
	log := log.FromContext(ctx)

	log.Info("creating deployer", "ctrlname", c.cfg.ControllerName, "server", c.cfg.ControlPlane.XdsHost, "port", c.cfg.ControlPlane.XdsPort)
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
				return c.cfg.GWClasses.Has(string(gw.Spec.GatewayClassName))
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
		kick:          c.cfg.Kick,
	}
	err = buildr.Complete(gwReconciler)
	if err != nil {
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

// watchCRDs sets up a controller to watch for changes to supported Gateway API CRDs
// and triggers GatewayClass reconciliation if generation or annotations change.
func (c *controllerBuilder) watchCRDs(_ context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		For(&apiextv1.CustomResourceDefinition{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			crd, ok := object.(*apiextv1.CustomResourceDefinition)
			if !ok {
				return false
			}
			// Check if the CRD is one we care about
			if !crds.IsSupported(crd.Name) {
				return false
			}
			// Maintain the set of supported CRDs
			if c.cfg.CRDs != nil {
				annotations, exist := (*c.cfg.CRDs)[crd.Name]
				if !exist {
					(*c.cfg.CRDs)[crd.Name] = make(map[string]string)
				}
				if !maps.Equal(annotations, crd.Annotations) {
					(*c.cfg.CRDs)[crd.Name] = crd.Annotations
				}
			}
			return true
		})).
		WithEventFilter(predicate.Or(
			predicate.AnnotationChangedPredicate{},
			predicate.GenerationChangedPredicate{},
		)).
		Complete(reconcile.Func(c.reconciler.ReconcileCRDs))
}

func (c *controllerBuilder) watchHttpRoute(_ context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		For(&apiv1.HTTPRoute{}).
		Complete(reconcile.Func(c.reconciler.ReconcileHttpRoutes))
}

func (c *controllerBuilder) watchTcpRoute(ctx context.Context) error {
	log := log.FromContext(ctx)

	if c.cfg.CRDs == nil {
		log.Info("CRD annotations map not initialized; skipping TCPRoute controller setup")
	}
	if _, exists := (*c.cfg.CRDs)[crds.TCPRoute]; !exists {
		log.Info("TCPRoute type not registered in scheme; skipping TCPRoute controller setup")
		return nil
	}

	// Proceed to set up the controller for TCPRoute
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		For(&apiv1a2.TCPRoute{}).
		Complete(reconcile.Func(c.reconciler.ReconcileTcpRoutes))
}

func (c *controllerBuilder) watchReferenceGrant(_ context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		For(&apiv1beta1.ReferenceGrant{}).
		Complete(reconcile.Func(c.reconciler.ReconcileReferenceGrants))
}

func (c *controllerBuilder) watchNamespaces(_ context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		For(&corev1.Namespace{}).
		Complete(reconcile.Func(c.reconciler.ReconcileNamespaces))
}

func (c *controllerBuilder) watchHttpListenerOptions(_ context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		For(&sologatewayv1.HttpListenerOption{}).
		Complete(reconcile.Func(c.reconciler.ReconcileHttpListenerOptions))
}

func (c *controllerBuilder) watchListenerOptions(_ context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		For(&sologatewayv1.ListenerOption{}).
		Complete(reconcile.Func(c.reconciler.ReconcileListenerOptions))
}

func (c *controllerBuilder) watchRouteOptions(_ context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		For(&sologatewayv1.RouteOption{}).
		Complete(reconcile.Func(c.reconciler.ReconcileRouteOptions))
}

func (c *controllerBuilder) watchVirtualHostOptions(_ context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		For(&sologatewayv1.VirtualHostOption{}).
		Complete(reconcile.Func(c.reconciler.ReconcileVirtualHostOptions))
}

func (c *controllerBuilder) watchUpstreams(_ context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		For(&gloov1.Upstream{}).
		Complete(reconcile.Func(c.reconciler.ReconcileUpstreams))
}

func (c *controllerBuilder) watchServices(_ context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		For(&corev1.Service{}).
		Complete(reconcile.Func(c.reconciler.ReconcileServices))
}

// watchDirectResponses watches for DirectResponses and triggers
// reconciliation of the Gateway that references them.
func (c *controllerBuilder) watchDirectResponses(_ context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		For(&v1alpha1.DirectResponse{}).
		Complete(reconcile.Func(c.reconciler.ReconcileDirectResponses))
}

func (c *controllerBuilder) watchPods(ctx context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		For(&corev1.Pod{}).
		Complete(reconcile.Func(c.reconciler.ReconcilePods))
}

func (c *controllerBuilder) watchEndpointSlices(ctx context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		For(&discoveryv1.EndpointSlice{}).
		Complete(reconcile.Func(c.reconciler.ReconcileEndpointSlices))
}

func (c *controllerBuilder) watchSecrets(ctx context.Context) error {
	return ctrl.NewControllerManagedBy(c.cfg.Mgr).
		For(&corev1.Secret{}).
		Complete(reconcile.Func(c.reconciler.ReconcileSecrets))
}

type controllerReconciler struct {
	controllerName string
	cli            client.Client
	scheme         *runtime.Scheme
	kick           func(ctx context.Context)
	// crds is a map of supported Gateway API CRDs with the discovered annotations for each.
	crds *crds.CrdToAnnotation
}

func (r *controllerReconciler) ReconcileHttpListenerOptions(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// eventually reconcile only effected routes/listeners etc
	// https://github.com/solo-io/gloo/issues/9997.
	r.kick(ctx)
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileListenerOptions(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// eventually reconcile only effected routes/listeners etc
	// https://github.com/solo-io/gloo/issues/9997.
	r.kick(ctx)
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileRouteOptions(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// eventually reconcile only effected routes/listeners etc
	// https://github.com/solo-io/gloo/issues/9997.
	r.kick(ctx)
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileDirectResponses(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO(tim): eventually reconcile only effected routes.
	// https://github.com/solo-io/gloo/issues/9997.
	r.kick(ctx)
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileVirtualHostOptions(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// eventually reconcile only effected listeners etc
	// https://github.com/solo-io/gloo/issues/9997.
	r.kick(ctx)
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileUpstreams(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// eventually reconcile only effected listeners etc
	// https://github.com/solo-io/gloo/issues/9997.
	r.kick(ctx)
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileServices(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// eventually reconcile only effected listeners etc
	// https://github.com/solo-io/gloo/issues/9997.
	r.kick(ctx)
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcilePods(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// eventually reconcile only effected listeners etc
	r.kick(ctx)
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileEndpointSlices(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// eventually reconcile only effected listeners etc
	r.kick(ctx)
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileSecrets(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// eventually reconcile only effected listeners etc
	r.kick(ctx)
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileNamespaces(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// reconcile all gateways with namespace selector
	// https://github.com/solo-io/gloo/issues/9997.
	r.kick(ctx)
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileHttpRoutes(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO: consider finding impacted gateways and queue them
	// TODO: consider enabling this
	//	// reconcile this specific route:
	//	queries := query.NewData(r.cli, r.scheme)
	//	httproute.TranslateGatewayHTTPRouteRules(queries, hr, nil)

	r.kick(ctx)
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileTcpRoutes(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO: consider finding impacted gateways and queue them
	// TODO: consider enabling this
	//	// reconcile this specific route:
	//	queries := query.NewData(r.cli, r.scheme)
	//	httproute.TranslateGatewayHTTPRouteRules(queries, hr, nil)

	r.kick(ctx)
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileReferenceGrants(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// reconcile all things?! https://github.com/solo-io/gloo/issues/9997
	r.kick(ctx)
	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileCRDs(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("CustomResourceDefinition", req.Name)

	log.Info("reconciling CustomResourceDefinition")

	var gatewayClassList apiv1.GatewayClassList
	if err := r.cli.List(ctx, &gatewayClassList); err != nil {
		log.Error(err, "Failed to list GatewayClasses")
		return ctrl.Result{}, err
	}

	for _, gatewayClass := range gatewayClassList.Items {
		if gatewayClass.Spec.ControllerName == apiv1.GatewayController(r.controllerName) {
			req := ctrl.Request{
				NamespacedName: client.ObjectKey{
					Name: gatewayClass.Name,
				},
			}

			if _, err := r.ReconcileGatewayClasses(ctx, req); err != nil {
				log.Error(err, "Failed to reconcile GatewayClass", "name", gatewayClass.Name)
				return ctrl.Result{}, err
			}
		}
	}

	r.kick(ctx)

	log.Info("Reconciled CustomResourceDefinition")

	return ctrl.Result{}, nil
}

func (r *controllerReconciler) ReconcileGatewayClasses(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("GatewayClass", req.Name)

	gwclass := &apiv1.GatewayClass{}
	if err := r.cli.Get(ctx, req.NamespacedName, gwclass); err != nil {
		// NOTE: if this reconciliation is a result of a DELETE event, this err will be a NotFound,
		// therefore we will return a nil error here and thus skip any additional reconciliation below.
		// At the time of writing this comment, the retrieved GWClass object is only used to update the status,
		// so it should be fine to return here, because there's no status update needed on a deleted resource.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling GatewayClass")

	// Initialize the status conditions. No need to set LastTransitionTime since it's handled by SetStatusCondition.
	acceptedCondition := metav1.Condition{
		Type:               string(apiv1.GatewayClassConditionStatusAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(apiv1.GatewayClassReasonAccepted),
		Message:            "All dependencies have been met",
		ObservedGeneration: gwclass.Generation,
	}
	supportedCondition := metav1.Condition{
		Type:               string(apiv1.GatewayClassConditionStatusSupportedVersion),
		Status:             metav1.ConditionTrue,
		Reason:             string(apiv1.GatewayClassReasonSupportedVersion),
		Message:            "Gateway API CRDs are a supported version",
		ObservedGeneration: gwclass.Generation,
	}

	// Set status conditions based on observed CRD versions
	if !r.crdsSupportedVersion(ctx) {
		// Update the values of status conditions
		msg := fmt.Sprintf("Unsupported Gateway API CRDs detected. Supported versions are: %s",
			strings.Join(wellknown.SupportedVersions, ", "))
		acceptedCondition.Status = metav1.ConditionFalse
		acceptedCondition.Reason = string(apiv1.GatewayClassReasonUnsupportedVersion)
		acceptedCondition.Message = msg
		supportedCondition.Status = metav1.ConditionFalse
		supportedCondition.Reason = string(apiv1.GatewayClassReasonUnsupportedVersion)
		supportedCondition.Message = msg
	}

	// Set the status conditions
	meta.SetStatusCondition(&gwclass.Status.Conditions, acceptedCondition)
	meta.SetStatusCondition(&gwclass.Status.Conditions, supportedCondition)

	if err := r.cli.Status().Update(ctx, gwclass); err != nil {
		log.Error(err, "Failed to update GatewayClass status")
		return ctrl.Result{}, err
	}

	log.Info("Reconciled GatewayClass")

	return ctrl.Result{}, nil
}

// crdsSupportedVersion returns true if the "gateway.networking.k8s.io/bundle-version" annotation key is
// set to a supported version for each managed Gateway API CRD.
func (r *controllerReconciler) crdsSupportedVersion(ctx context.Context) bool {
	log := log.FromContext(ctx)

	if r.crds == nil {
		log.Info("supported CRDs are not initialized by the controller")
		return false
	}

	for crdName, annotations := range *r.crds {
		if annotations == nil || len(annotations) == 0 {
			log.Info("no annotations found for CRD", "name", crdName)
			return false
		}

		bundleVersion, exists := annotations[consts.BundleVersionAnnotation]
		if !exists {
			log.Info("bundle version annotation does not exist for CRD", "name", crdName)
			return false
		}
		if !crds.IsSupportedVersion(bundleVersion) {
			log.Info("bundle version annotation not a supported version for CRD", "name", crdName, "version", bundleVersion)
			return false
		}
	}

	return true
}
