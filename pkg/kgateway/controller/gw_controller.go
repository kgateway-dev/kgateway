package controller

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"unicode/utf8"

	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/events"
	utilretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	internaldeployer "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

const (
	GatewayAutoDeployAnnotationKey = "gateway.kgateway.dev/auto-deploy"

	// maxEventNoteLength is the most bytes the API server accepts in the note of an
	// events.k8s.io event. It rejects an event with a longer one.
	maxEventNoteLength = 1024
	// maxConditionMessageLength is the most characters the API server accepts in a
	// condition's message. It rejects a status update with a longer one.
	maxConditionMessageLength = 32768
	// maxOverlayErrorLength is the most bytes of a bootstrap overlay error that go into a
	// condition message. Envoy names the field it rejects at the start of its error and
	// can follow it with the whole rejected message.
	maxOverlayErrorLength = 4096
	// truncatedSuffix ends a text that truncateText cut.
	truncatedSuffix = " ... (truncated; the controller log has the full text)"
)

var logger = logging.New("gateway-controller")

var _ manager.LeaderElectionRunnable = (*gatewayReconciler)(nil)

type gatewayReconciler struct {
	ctx            context.Context
	deployer       *deployer.Deployer
	gwParams       *internaldeployer.GatewayParameters
	scheme         *runtime.Scheme
	controllerName string

	gwClient         kclient.Client[*gwv1.Gateway]
	gwClassClient    kclient.Client[*gwv1.GatewayClass]
	gwParamClient    kclient.Client[*kgateway.GatewayParameters]
	nsClient         kclient.Client[*corev1.Namespace]
	svcClient        kclient.Client[*corev1.Service]
	deploymentClient kclient.Client[*appsv1.Deployment]
	svcAccountClient kclient.Client[*corev1.ServiceAccount]
	configMapClient  kclient.Client[*corev1.ConfigMap]

	controllerExtension pluginsdk.GatewayControllerExtension

	// recorder records the events the deployer reports on Gateways.
	recorder events.EventRecorder

	// runningBootstraps is what each Gateway's proxy pods say about the bootstraps
	// they run, which the deployer reads to keep their bootstrap revisions; nil when
	// the controller does not collect it.
	runningBootstraps krt.Collection[krtcollections.GatewayRunningBootstraps]

	// paramsStatus is where Reconcile reports whether each Gateway's objects could be built
	// from its parameters. The Gateway status writer turns a failure into
	// Accepted=False/InvalidParameters; this controller never writes that condition itself.
	paramsStatus *reports.GatewayParametersStatus

	queue controllers.Queue
}

func NewGatewayReconciler(
	cfg GatewayConfig,
	deployer *deployer.Deployer,
	gwParams *internaldeployer.GatewayParameters,
	controllerExtension pluginsdk.GatewayControllerExtension,
) *gatewayReconciler {
	filter := kclient.Filter{ObjectFilter: cfg.Client.ObjectFilter()}
	r := &gatewayReconciler{
		deployer:            deployer,
		gwParams:            gwParams,
		scheme:              cfg.Mgr.GetScheme(),
		controllerName:      cfg.ControllerName,
		controllerExtension: controllerExtension,
		recorder:            cfg.Mgr.GetEventRecorder(cfg.ControllerName),

		gwClient:         kclient.NewFilteredDelayed[*gwv1.Gateway](cfg.Client, gvr.KubernetesGateway, filter),
		gwClassClient:    kclient.NewFilteredDelayed[*gwv1.GatewayClass](cfg.Client, gvr.GatewayClass, filter),
		nsClient:         kclient.NewFiltered[*corev1.Namespace](cfg.Client, filter),
		svcClient:        kclient.NewFiltered[*corev1.Service](cfg.Client, filter),
		deploymentClient: kclient.NewFiltered[*appsv1.Deployment](cfg.Client, filter),
		svcAccountClient: kclient.NewFiltered[*corev1.ServiceAccount](cfg.Client, filter),
		configMapClient:  kclient.NewFiltered[*corev1.ConfigMap](cfg.Client, filter),
	}

	// Reuse the parameter client from the deployer to avoid duplicate watches.
	r.gwParamClient = gwParams.GetGatewayParametersClient()

	r.queue = controllers.NewQueue("GatewayController", controllers.WithReconciler(r.Reconcile), controllers.WithMaxAttempts(math.MaxInt), controllers.WithRateLimiter(rateLimiter))
	r.paramsStatus = cfg.GatewayParametersStatus

	// Gateway event handler
	r.gwClient.AddEventHandler(
		controllers.FromEventHandler(func(o controllers.Event) {
			switch o.Event {
			case controllers.EventAdd:
				logger.Debug("reconciling Gateway due to add event", "ref", kubeutils.NamespacedNameFrom(o.New))
				r.queue.AddObject(o.New)
			case controllers.EventUpdate:
				if o.New.GetGeneration() != o.Old.GetGeneration() {
					logger.Debug("reconciling Gateway due to generation change", "ref", kubeutils.NamespacedNameFrom(o.New))
					r.queue.AddObject(o.New)
					break
				}
				// TODO: check if we want to reconcile when labels change
				if !maps.Equal(o.New.GetAnnotations(), o.Old.GetAnnotations()) {
					logger.Debug("reconciling Gateway due to annotation change", "ref", kubeutils.NamespacedNameFrom(o.New))
					r.queue.AddObject(o.New)
					break
				}
				logger.Debug("skip reconciling Gateway with no relevant changes", "ref", kubeutils.NamespacedNameFrom(o.New))
			case controllers.EventDelete:
				logger.Debug("reconciling Gateway due to delete event", "ref", kubeutils.NamespacedNameFrom(o.Old))
				r.queue.AddObject(o.Old)
			}
		}))

	// GatewayClass event handler
	r.gwClassClient.AddEventHandler(controllers.ObjectHandler(func(o controllers.Object) {
		gwClass, ok := o.(*gwv1.GatewayClass)
		if !ok {
			// should not happen
			logger.Error("got unexpected type instead of GatewayClass", "kind", o.GetObjectKind())
			return
		}
		// If this GatewayClass is not ours, ignore it
		if gwClass.Spec.ControllerName != gwv1.GatewayController(r.controllerName) || wellknown.IsRetiredWaypointClass(gwClass) {
			return
		}
		for _, g := range r.gwClient.List(metav1.NamespaceAll, labels.Everything()) {
			if string(g.Spec.GatewayClassName) == o.GetName() {
				logger.Debug("reconciling Gateway due to GatewayClass change",
					"ref", kubeutils.NamespacedNameFrom(g), "gwclass", kubeutils.NamespacedNameFrom(gwClass))
				r.queue.AddObject(g)
			}
		}
	}))

	// GatewayParameters event handler
	gatewaysByParamsRef := kclient.CreateIndex(r.gwClient, "parametersRef", func(o *gwv1.Gateway) []types.NamespacedName {
		p := fetchGatewaysByParametersRef(o)
		if p == nil {
			return nil
		}
		return []types.NamespacedName{*p}
	})
	gatewaysByClass := kclient.CreateIndex(r.gwClient, "gatewayClass", func(o *gwv1.Gateway) []types.NamespacedName {
		p := fetchGatewaysByGatewayClass(o)
		return []types.NamespacedName{p}
	})
	// gwParamEventHandler is a handler that reconciles Gateways based on GatewayParameters changes
	gwParamEventHandler := controllers.ObjectHandler(func(o controllers.Object) {
		gwpName := o.GetName()
		gwpNamespace := o.GetNamespace()

		// 1. Look up Gateways directly using this GatewayParameters object (via spec.infrastructure.parametersRef)
		gateways := gatewaysByParamsRef.Lookup(types.NamespacedName{Namespace: gwpNamespace, Name: gwpName})
		for _, gw := range gateways {
			logger.Debug("reconciling Gateway due to GatewayParameters change",
				"ref", kubeutils.NamespacedNameFrom(gw), "gwparam", types.NamespacedName{Namespace: gwpNamespace, Name: gwpName})
			r.queue.AddObject(gw)
		}

		// 2. Look up GatewayClasses using this GatewayParameters object (via spec.parametersRef)
		gwClasses := r.gwClassClient.List(metav1.NamespaceAll, labels.Everything())
		// For each GatewayClass that references this parameter, find all Gateways using that class
		for _, gc := range gwClasses {
			// Only process GatewayClasses managed by our controller
			if gc.Spec.ControllerName != gwv1.GatewayController(r.controllerName) || wellknown.IsRetiredWaypointClass(gc) {
				continue
			}
			if gc.Spec.ParametersRef != nil &&
				gc.Spec.ParametersRef.Name == gwpName &&
				gc.Spec.ParametersRef.Namespace != nil && string(*gc.Spec.ParametersRef.Namespace) == gwpNamespace {
				// This GatewayClass references our GatewayParameters, find all Gateways using this class
				gateways := gatewaysByClass.Lookup(types.NamespacedName{Name: gc.Name})
				for _, gw := range gateways {
					logger.Debug("reconciling Gateway due to GatewayParameters change via GatewayClass",
						"ref", kubeutils.NamespacedNameFrom(gw), "gwparam", types.NamespacedName{Namespace: gwpNamespace, Name: gwpName},
						"gwclass", gc.Name)
					r.queue.AddObject(gw)
				}
			}
		}
	})
	r.gwParamClient.AddEventHandler(gwParamEventHandler)

	// Custom event handler for ListenerSet changes
	cfg.CommonCollections.GatewayIndex.GatewaysForDeployer.Register(func(o krt.Event[ir.GatewayForDeployer]) {
		gw := o.Latest()
		ref := types.NamespacedName{Namespace: gw.Namespace, Name: gw.Name}
		logger.Debug("reconciling Gateway due to ListenerSet change", "ref", ref)
		r.queue.Add(ref)
	})

	// Reconcile a Gateway when the bootstraps its proxy pods run change: a bootstrap
	// revision no pod uses can be removed from the generated ConfigMap.
	if running := cfg.CommonCollections.GatewayIndex.RunningBootstraps; running != nil {
		r.runningBootstraps = running
		running.Register(func(o krt.Event[krtcollections.GatewayRunningBootstraps]) {
			ref := o.Latest().Gateway
			logger.Debug("reconciling Gateway due to a change in the bootstraps its proxies run", "ref", ref)
			r.queue.Add(ref)
		})
	}

	// Add a handler to reconcile the parent Gateway when child objects (Deployment, Service, etc.)
	enqueueParent := controllers.EnqueueForParentHandler(r.queue, gvk.KubernetesGateway)
	parentHandler := controllers.ObjectHandler(enqueueParent)
	r.deploymentClient.AddEventHandler(parentHandler)
	r.svcAccountClient.AddEventHandler(parentHandler)
	r.svcClient.AddEventHandler(parentHandler)
	r.configMapClient.AddEventHandler(parentHandler)

	// Reconcile the Gateway that owns a ReplicaSet's Deployment when the ReplicaSet
	// changes, so a bootstrap revision is removed once no ReplicaSet can create a pod with it.
	r.gwParams.GetReplicaSetClient().AddEventHandler(controllers.ObjectHandler(func(o controllers.Object) {
		owner := metav1.GetControllerOf(o)
		if owner == nil || owner.Kind != wellknown.DeploymentGVK.Kind {
			return
		}
		if dep := r.deploymentClient.Get(owner.Name, o.GetNamespace()); dep != nil {
			enqueueParent(dep)
		}
	}))

	// Register controller extensions
	if controllerExtension != nil {
		controllerExtension.Register(r.queue, gwParamEventHandler)
	}

	// Add a handler to reconcile the Gateways when the xDS TLS certificate changes
	r.setupTLSCertificateWatch(cfg.CertWatcher)

	return r
}

// NeedLeaderElection returns true to ensure that the Gateway reconciler runs only on the leader
func (r *gatewayReconciler) NeedLeaderElection() bool {
	return true
}

// Start starts the Gateway reconciler and blocks until the stop channel is closed.
func (r *gatewayReconciler) Start(ctx context.Context) error {
	r.ctx = ctx
	// Add all clients handlers on gatewayReconciler
	hasSynced := []cache.InformerSynced{
		r.gwClient.HasSynced,
		r.gwClassClient.HasSynced,
		r.nsClient.HasSynced,
		r.deploymentClient.HasSynced,
		r.svcAccountClient.HasSynced,
		r.svcClient.HasSynced,
		r.configMapClient.HasSynced,
	}
	// Add GatewayParameters cache sync handlers
	hasSynced = append(hasSynced, r.gwParams.GetCacheSyncHandlers()...)

	// Wait for the running bootstraps too: the deployer removes a bootstrap revision
	// no pod runs, so a reconcile before the pods are known would read them as none.
	if r.runningBootstraps != nil {
		hasSynced = append(hasSynced, r.runningBootstraps.HasSynced)
	}

	// Wait for all caches to sync
	kube.WaitForCacheSync("GatewayController", ctx.Done(), hasSynced...)
	if r.controllerExtension != nil {
		r.controllerExtension.Start(ctx)
	}
	r.queue.Run(ctx.Done())

	// Shutdown all the clients
	clients := []controllers.Shutdowner{
		r.gwClient,
		r.gwClassClient,
		r.gwParamClient,
		r.gwParams.GetReplicaSetClient(),
		r.nsClient,
		r.deploymentClient,
		r.svcAccountClient,
		r.svcClient,
		r.configMapClient,
	}
	controllers.ShutdownAll(clients...)
	if r.controllerExtension != nil {
		r.controllerExtension.Stop()
	}
	return nil
}

func (r *gatewayReconciler) Reconcile(req types.NamespacedName) (rErr error) {
	defer func() {
		recordReconciliation("gateway", rErr)
	}()

	gw := r.gwClient.Get(req.Name, req.Namespace)
	if gw == nil || gw.GetDeletionTimestamp() != nil {
		// ignore the event if the Gateway is not found. A subsequent event should handle this if needed
		logger.Debug("gateway not found, skipping reconciliation", "ref", req)
		internaldeployer.ForgetBootstrapOverlayMetrics(req)
		r.paramsStatus.Remove(req)
		return nil
	}

	// make sure we're the right controller for this
	gwc := r.gwClassClient.Get(string(gw.Spec.GatewayClassName), "")
	if gwc == nil {
		return fmt.Errorf("gatewayclass %s not found for gateway %s", gw.Spec.GatewayClassName, req)
	}

	// Only reconcile Gateways for our controller
	if gwc.Spec.ControllerName != gwv1.GatewayController(r.controllerName) {
		// Not our GatewayClass at all
		r.paramsStatus.Remove(req)
		return nil
	}
	if wellknown.IsRetiredWaypointClass(gwc) {
		logger.Debug("ignoring Gateway of a retired waypoint GatewayClass", "ref", req, "gwclass", gwc.Name)
		r.paramsStatus.Remove(req)
		return nil
	}

	logger.Info("reconciling Gateway", "ref", req)
	ctx := r.ctx
	objs, report, err := r.deployer.GetObjsToDeployWithReport(ctx, gw)
	if errors.Is(err, internaldeployer.ErrNoValidPorts) {
		// ErrNoValidPorts is only returned once the GatewayParameters resolve, so the
		// parameters are valid; the listener ports' own status is reported from the
		// translator.
		r.paramsStatus.Set(req, gw.Generation, nil)
		return err
	}
	if err != nil {
		if ctx.Err() != nil {
			// The controller is stopping; the error says nothing about the Gateway.
			return err
		}
		if errors.Is(err, internaldeployer.ErrBootstrapConfigMapUnreadable) {
			// A failed read from the API server says nothing about the Gateway, so its
			// status and objects are left as they are and the reconcile is retried.
			logger.Info("not deploying Gateway: could not read its generated bootstrap ConfigMap; retrying", "ref", req, "error", err)
			return err
		}
		if overlayErr, ok := errors.AsType[*internaldeployer.BootstrapOverlayError](err); ok {
			// Log the whole error, and report its start: the conditions keep room for
			// what is added after it.
			logger.Info("not deploying Gateway: its bootstrap overlay was not applied", "ref", req, "reason", overlayErr.Reason, "retry", overlayErr.Retry, "error", overlayErr)
			overlayMsg := truncateText(overlayErr.Error(), maxOverlayErrorLength)
			cond := metav1.Condition{
				Type:               conditions.KgatewayConditionBootstrapOverlayApplied,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: gw.Generation,
				Reason:             overlayErr.Reason,
				Message:            overlayMsg,
			}

			// Say when the overlay could not reach the proxy anyway, so removing it
			// deploys the held-back objects.
			if note := overlayErr.MountNote(); note != "" {
				cond.Message += " " + note
			}

			// A Gateway whose bootstrap ConfigMap does not exist yet has no proxy, so it
			// is not accepted. An existing proxy keeps running its last applied
			// bootstrap, so the overlay failure does not count against the Gateway's
			// parameters: the verdict on them, and with it Accepted, is the result of
			// post-processing the objects, which replaces a failure stored by an earlier
			// reconcile of the same generation. Without the ConfigMap's name, which
			// only a controller defect leaves out, the verdict is left alone.
			evts := overlayErr.Events()
			hasProxy := true
			switch {
			case overlayErr.ConfigMapName == "":
			case r.configMapClient.Get(overlayErr.ConfigMapName, gw.Namespace) == nil:
				hasProxy = false
				r.paramsStatus.Set(req, gw.Generation, errors.New("No proxy is deployed until the bootstrap overlay applies: "+overlayMsg))
			default:
				r.paramsStatus.Set(req, gw.Generation, overlayErr.ObjectsErr)
			}

			// The existing proxy keeps its image too, which can hold back an Envoy
			// upgrade; name both images.
			if heldBack := r.heldBackProxyImage(gw, overlayErr.Proxy); hasProxy && heldBack != "" {
				cond.Message += " " + heldBack
				evts = append(evts, internaldeployer.HeldBackProxyImageEvent(heldBack))
			}
			if statusErr := r.applyGatewayConditions(ctx, gw, []metav1.Condition{cond}, nil, evts); statusErr != nil {
				return fmt.Errorf("failed to update status for Gateway %s: %w", req, statusErr)
			}

			// Retry only when applying the overlays again can succeed. Otherwise a change
			// to the GatewayParameters or GatewayClass starts a new reconcile, and so
			// does the controller restart that a change to its Deployment or settings causes.
			if overlayErr.Retry {
				return overlayErr
			}
			return nil
		}
		if tooLarge, ok := errors.AsType[*internaldeployer.BootstrapConfigMapTooLargeError](err); ok {
			// A Gateway without bootstrap overlays has no BootstrapOverlayApplied condition,
			// so the failure is reported as an event, every time the reconcile hits it.
			// The objects are left as they are; the pod and ReplicaSet changes that free
			// earlier revisions start a new reconcile.
			logger.Info("not deploying Gateway: its generated bootstrap ConfigMap would be too large", "ref", req, "error", tooLarge)
			if statusErr := r.applyGatewayConditions(ctx, gw, nil, []string{conditions.KgatewayConditionBootstrapOverlayApplied}, nil); statusErr != nil {
				return fmt.Errorf("failed to update status for Gateway %s: %w", req, statusErr)
			}
			evts := tooLarge.Events()
			if heldBack := r.heldBackProxyImage(gw, tooLarge.Proxy); heldBack != "" {
				evts = append(evts, internaldeployer.HeldBackProxyImageEvent(heldBack))
			}
			r.recordEvents(gw, evts)
			return nil
		}

		// if we fail to either reference a valid GatewayParameters or
		// the GatewayParameters configuration leads to issues building the
		// objects, we want to set the status to InvalidParameters. Nothing says
		// whether a bootstrap overlay would apply, so its condition is removed.
		r.paramsStatus.Set(req, gw.Generation, err)
		if statusErr := r.applyGatewayConditions(ctx, gw, nil, []string{conditions.KgatewayConditionBootstrapOverlayApplied}, nil); statusErr != nil {
			return fmt.Errorf("failed to update status for Gateway %s: %w", req, statusErr)
		}
		return err
	}
	r.paramsStatus.Set(req, gw.Generation, nil)
	objs = r.deployer.SetNamespaceAndOwnerWithGVK(gw, wellknown.GatewayGVK, objs)
	err = r.deployer.DeployObjsWithSource(ctx, objs, gw)
	if err != nil {
		return err
	}

	// Prune any PDB/HPA/VPA resources that are no longer desired
	err = r.deployer.PruneRemovedResources(ctx, gw, objs)
	if err != nil {
		return fmt.Errorf("error pruning removed resources for Gateway %s: %w", req, err)
	}

	// Report what the post-processor found, now that the objects are deployed. Without
	// a report, as for a self-managed Gateway, kgateway deploys no proxy for the
	// Gateway, so a bootstrap overlay condition and metric from before are removed.
	switch {
	case report != nil:
		if err := r.applyGatewayConditions(ctx, gw, report.Conditions, report.OwnedConditionTypes, report.Events); err != nil {
			return fmt.Errorf("error updating status for Gateway %s: %w", req, err)
		}
	case meta.FindStatusCondition(gw.Status.Conditions, conditions.KgatewayConditionBootstrapOverlayApplied) != nil:
		internaldeployer.ForgetBootstrapOverlayMetrics(req)
		if err := r.applyGatewayConditions(ctx, gw, nil, []string{conditions.KgatewayConditionBootstrapOverlayApplied}, nil); err != nil {
			return fmt.Errorf("error updating status for Gateway %s: %w", req, err)
		}
	}

	// find the name/ns of the service we own so we can grab addresses
	// from it for status
	var generatedSvc *metav1.ObjectMeta
	for _, obj := range objs {
		if svc, ok := obj.(*corev1.Service); ok {
			generatedSvc = &svc.ObjectMeta
			break
		}
	}
	// update status (whether we generated a service or not, for unmanaged)
	err = r.updateStatus(ctx, gw, generatedSvc)
	if err != nil {
		return fmt.Errorf("error updating status for Gateway %s: %w", req, err)
	}

	return nil
}

func (r *gatewayReconciler) updateStatus(ctx context.Context, gw *gwv1.Gateway, svcMeta *metav1.ObjectMeta) error {
	var svc *corev1.Service
	if svcMeta != nil {
		svcnns := client.ObjectKey{
			Namespace: svcMeta.Namespace,
			Name:      svcMeta.Name,
		}

		svc = r.svcClient.Get(svcnns.Name, svcnns.Namespace)
		if svc == nil {
			return fmt.Errorf("Service %s not found for Gateway %s", svcnns, kubeutils.NamespacedNameFrom(gw))
		}

		// make sure we own this service
		controller := metav1.GetControllerOf(svc)
		if controller == nil {
			return nil
		}

		if gw.UID != controller.UID {
			return nil
		}
	}

	// update gateway addresses in the status
	desiredAddresses := getDesiredAddresses(gw, svc)
	return updateGatewayAddresses(ctx, r.gwClient, client.ObjectKeyFromObject(gw), desiredAddresses)
}

func getDesiredAddresses(gw *gwv1.Gateway, svc *corev1.Service) []gwv1.GatewayStatusAddress {
	var ret []gwv1.GatewayStatusAddress
	seen := sets.New[gwv1.GatewayStatusAddress]()

	if svc != nil && svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if len(svc.Status.LoadBalancer.Ingress) == 0 {
			return nil
		}
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if addr, ok := convertIngressAddr(ing); ok {
				seen.Insert(addr)
				ret = append(ret, addr)
			}
		}

		return ret
	} else if svc != nil {
		t := gwv1.IPAddressType
		if len(svc.Spec.ClusterIPs) != 0 {
			for _, ip := range svc.Spec.ClusterIPs {
				ret = append(ret, gwv1.GatewayStatusAddress{
					Type:  &t,
					Value: ip,
				})
			}
		} else if svc.Spec.ClusterIP != "" {
			ret = append(ret, gwv1.GatewayStatusAddress{
				Type:  &t,
				Value: svc.Spec.ClusterIP,
			})
		}
	}

	for _, specAddr := range gw.Spec.Addresses {
		addr := gwv1.GatewayStatusAddress{
			Type:  specAddr.Type,
			Value: specAddr.Value,
		}
		if !seen.Has(addr) {
			ret = append(ret, addr)
		}
	}

	return ret
}

// updateGatewayStatusWithRetryFunc updates a Gateway's status with retry logic.
// The updateFunc receives the latest Gateway and should modify its status as needed.
// If updateFunc returns false, the update is skipped (no changes needed).
func updateGatewayStatusWithRetryFunc(
	_ context.Context,
	cli kclient.Client[*gwv1.Gateway],
	gwNN types.NamespacedName,
	updateFunc func(*gwv1.Gateway) (gwv1.GatewayStatus, bool),
) error {
	err := utilretry.RetryOnConflict(utilretry.DefaultRetry, func() error {
		gw := cli.Get(gwNN.Name, gwNN.Namespace)
		if gw == nil {
			// If the Gateway no longer exists, there's nothing to update.
			logger.Warn("gateway not found during status update", "ref", gwNN)
			return nil
		}
		status, needsUpdate := updateFunc(gw)
		if !needsUpdate {
			return nil
		}
		_, err := cli.UpdateStatus(&gwv1.Gateway{
			ObjectMeta: pluginsdk.CloneObjectMetaForStatus(gw.ObjectMeta),
			Status:     status,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to update gateway status: %w", err)
	}

	return nil
}

// updateGatewayAddresses updates the addresses of a Gateway resource.
func updateGatewayAddresses(
	ctx context.Context,
	cli kclient.Client[*gwv1.Gateway],
	gwNN types.NamespacedName,
	desired []gwv1.GatewayStatusAddress,
) error {
	return updateGatewayStatusWithRetryFunc(
		ctx,
		cli,
		gwNN,
		func(gw *gwv1.Gateway) (gwv1.GatewayStatus, bool) {
			// Check if an update is needed
			if slices.Equal(desired, gw.Status.Addresses) {
				return gw.Status, false
			}
			newStatus := gw.Status.DeepCopy()
			newStatus.Addresses = desired
			return *newStatus, true
		},
	)
}

// applyGatewayConditions sets each condition on the Gateway's status, and removes each
// type in ownedTypes that conds does not set. It writes the status only when it
// changes, and records evts on the Gateway only when a condition's status, reason or
// message changes, so that reconciling an unchanged Gateway, or one whose edit only
// moves the observed generation, does not repeat them.
func (r *gatewayReconciler) applyGatewayConditions(
	ctx context.Context,
	gw *gwv1.Gateway,
	conds []metav1.Condition,
	ownedTypes []string,
	evts []deployer.ReportEvent,
) error {
	// Keep each message within the API server's limit, so that a long one does not
	// fail the status update. The stored message is the cut one, so cutting before the
	// comparison below keeps an unchanged condition from reading as changed.
	conds = slices.Clone(conds)
	for i := range conds {
		if cut := truncateText(conds[i].Message, maxConditionMessageLength); cut != conds[i].Message {
			logger.Info("truncated a Gateway condition message to the API server's limit", "ref", client.ObjectKeyFromObject(gw), "type", conds[i].Type, "message", conds[i].Message)
			conds[i].Message = cut
		}
	}

	// Update the status. The last attempt decides whether it changed, and whether a
	// condition says something new.
	var changed, eventsDue bool
	err := updateGatewayStatusWithRetryFunc(
		ctx,
		r.gwClient,
		client.ObjectKeyFromObject(gw),
		func(latest *gwv1.Gateway) (gwv1.GatewayStatus, bool) {
			// Set the reported conditions.
			newStatus := latest.Status.DeepCopy()
			changed, eventsDue = false, false
			for _, c := range conds {
				eventsDue = conditionDiffers(latest.Status.Conditions, c) || eventsDue
				changed = meta.SetStatusCondition(&newStatus.Conditions, c) || changed
			}

			// Remove the owned conditions that were not reported.
			for _, t := range ownedTypes {
				if !slices.ContainsFunc(conds, func(c metav1.Condition) bool { return c.Type == t }) {
					changed = meta.RemoveStatusCondition(&newStatus.Conditions, t) || changed
				}
			}
			return *newStatus, changed
		},
	)
	if err != nil || !eventsDue {
		return err
	}

	// Record the events that go with the new status.
	r.recordEvents(gw, evts)
	return nil
}

// conditionDiffers reports whether c differs in status, reason or message from the
// condition of its type in existing, or existing has none of that type.
func conditionDiffers(existing []metav1.Condition, c metav1.Condition) bool {
	old := meta.FindStatusCondition(existing, c.Type)
	return old == nil || old.Status != c.Status || old.Reason != c.Reason || old.Message != c.Message
}

// heldBackProxyImage returns the sentence that names the image gw's live proxy
// Deployment keeps running and the image of rendered, which was not deployed, or ""
// when they are the same or either is unknown.
func (r *gatewayReconciler) heldBackProxyImage(gw *gwv1.Gateway, rendered internaldeployer.RenderedProxy) string {
	if rendered.DeploymentName == "" {
		return ""
	}
	return internaldeployer.HeldBackProxyImage(r.deploymentClient.Get(rendered.DeploymentName, gw.Namespace), rendered)
}

// recordEvents records evts on gw. A note over the API server's limit is cut to it and
// logged whole, because the API server rejects the event otherwise.
func (r *gatewayReconciler) recordEvents(gw *gwv1.Gateway, evts []deployer.ReportEvent) {
	for _, e := range evts {
		note := truncateText(e.Note, maxEventNoteLength)
		if note != e.Note {
			logger.Info("truncated a Gateway event note to the API server's limit", "ref", client.ObjectKeyFromObject(gw), "reason", e.Reason, "note", e.Note)
		}
		r.recorder.Eventf(gw, nil, e.Type, e.Reason, e.Action, "%s", note)
	}
}

// truncateText returns s when it is at most limit bytes, and otherwise its start, cut
// on a character boundary, followed by truncatedSuffix, limit bytes at most in all.
func truncateText(s string, limit int) string {
	// Return a text within the limit as it is.
	if len(s) <= limit {
		return s
	}

	// Cut before the first byte of a character, leaving room for the suffix.
	cut := max(limit-len(truncatedSuffix), 0)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + truncatedSuffix
}

// setupTLSCertificateWatch configures a watch for xDS TLS certificate changes.
// When certificates are rotated, all Gateways managed by this controller will be reconciled
// to update the proxy CA certificates.
func (r *gatewayReconciler) setupTLSCertificateWatch(certWatcher *certwatcher.CertWatcher) {
	if certWatcher == nil {
		return
	}

	// Register callback to send events when certificate changes
	certWatcher.RegisterCallback(func(_ tls.Certificate) {
		logger.Info("xDS TLS certificate changed, triggering Gateway reconciliation")
		gateways := r.gwClient.List(metav1.NamespaceAll, labels.Everything())
		for _, gw := range gateways {
			gwClass := r.gwClassClient.Get(string(gw.Spec.GatewayClassName), "")
			ref := kubeutils.NamespacedNameFrom(gw)
			if gwClass == nil {
				logger.Error("error getting GatewayClass for Gateway during certificate change", "ref", ref)
				continue
			}
			if gwClass.Spec.ControllerName == gwv1.GatewayController(r.controllerName) && !wellknown.IsRetiredWaypointClass(gwClass) {
				logger.Debug("enqueueing Gateway for reconciliation due to certificate change", "ref", ref)
				r.queue.AddObject(gw)
			}
		}
	})
}

func convertIngressAddr(ing corev1.LoadBalancerIngress) (gwv1.GatewayStatusAddress, bool) {
	if ing.Hostname != "" {
		t := gwv1.HostnameAddressType
		return gwv1.GatewayStatusAddress{
			Type:  &t,
			Value: ing.Hostname,
		}, true
	}
	if ing.IP != "" {
		t := gwv1.IPAddressType
		return gwv1.GatewayStatusAddress{
			Type:  &t,
			Value: ing.IP,
		}, true
	}
	return gwv1.GatewayStatusAddress{}, false
}

func fetchGatewaysByParametersRef(
	gw *gwv1.Gateway,
) *types.NamespacedName {
	if gw.Spec.Infrastructure != nil && gw.Spec.Infrastructure.ParametersRef != nil {
		pr := gw.Spec.Infrastructure.ParametersRef
		return &types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      pr.Name,
		}
	}
	return nil
}

func fetchGatewaysByGatewayClass(gw *gwv1.Gateway) types.NamespacedName {
	return types.NamespacedName{
		Name: string(gw.Spec.GatewayClassName),
	}
}
