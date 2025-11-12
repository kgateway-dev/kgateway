package controller

import (
	"context"
	"errors"
	"fmt"

	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

var _ manager.LeaderElectionRunnable = (*gatewayClassReconciler)(nil)

// emptyGatewayClass is a sentinel value for an empty GatewayClass used to create
// the default GatewayClass
var emptyGatewayClass = types.NamespacedName{}

type gatewayClassReconciler struct {
	classInfo             map[string]*deployer.GatewayClassInfo
	defaultControllerName string
	gwClassClient         kclient.Client[*gwv1.GatewayClass]
	queue                 controllers.Queue
}

func newGatewayClassReconciler(
	cfg GatewayConfig,
	classInfo map[string]*deployer.GatewayClassInfo,
) *gatewayClassReconciler {
	filter := kclient.Filter{ObjectFilter: cfg.Client.ObjectFilter()}
	r := &gatewayClassReconciler{
		defaultControllerName: cfg.ControllerName,
		classInfo:             classInfo,
		gwClassClient:         kclient.NewFilteredDelayed[*gwv1.GatewayClass](cfg.Client, gvr.GatewayClass_v1, filter),
	}
	r.queue = controllers.NewQueue("GatewayClassController", controllers.WithReconciler(r.reconcile), controllers.WithMaxAttempts(10))
	ourControllers := sets.New(cfg.ControllerName, cfg.AgwControllerName)

	r.gwClassClient.AddEventHandler(
		controllers.FromEventHandler(func(o controllers.Event) {
			switch o.Event {
			case controllers.EventAdd:
				logger.Debug("reconciling Gateway due to add event", "ref", kubeutils.NamespacedNameFrom(o.New))
				if isOurGatewayClass(o.New.(*gwv1.GatewayClass), ourControllers) {
					r.queue.AddObject(o.New)
				}
			case controllers.EventUpdate:
				if o.New.GetGeneration() != o.Old.GetGeneration() && isOurGatewayClass(o.New.(*gwv1.GatewayClass), ourControllers) {
					logger.Debug("reconciling Gateway due to generation change", "ref", kubeutils.NamespacedNameFrom(o.New))
					r.queue.AddObject(o.New)
					return
				}
				logger.Debug("skip reconciling Gateway with no relevant changes", "ref", kubeutils.NamespacedNameFrom(o.New))
			case controllers.EventDelete:
				logger.Debug("reconciling Gateway due to delete event", "ref", kubeutils.NamespacedNameFrom(o.Old))
				r.queue.Add(emptyGatewayClass)
			}
		}))

	return r
}

// NeedLeaderElection returns true to ensure that the GatewayClass runs only on the leader
func (r *gatewayClassReconciler) NeedLeaderElection() bool {
	return true
}

// Start starts the GatewayClass reconciler and blocks until the stop channel is closed.
func (r *gatewayClassReconciler) Start(ctx context.Context) error {
	// Seed the queue with an initial event to ensure default GatewayClass creation
	r.queue.Add(emptyGatewayClass)

	// Wait for all caches to sync
	kube.WaitForCacheSync("GatewayClassController", ctx.Done(), r.gwClassClient.HasSynced)
	r.queue.Run(ctx.Done())

	// Shutdown all the clients
	controllers.ShutdownAll(r.gwClassClient)
	return nil
}

func (r *gatewayClassReconciler) reconcile(req types.NamespacedName) (rErr error) {
	finishMetrics := collectReconciliationMetrics("gatewayclass", req)
	defer func() {
		finishMetrics(rErr)
	}()

	if req == emptyGatewayClass {
		// This is the initial reconciliation event to ensure the default GatewayClasses are created
		return r.createGatewayClasses()
	}

	gwClass := r.gwClassClient.Get(req.Name, req.Namespace)
	if gwClass == nil || gwClass.GetDeletionTimestamp() != nil {
		logger.Debug("gatewayclass not found, skipping reconciliation", "ref", req)
		return nil
	}

	status := gwClass.Status
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               string(gwv1.GatewayClassConditionStatusAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(gwv1.GatewayClassReasonAccepted),
		ObservedGeneration: gwClass.Generation,
		Message:            reports.GatewayClassAcceptedMessage,
	})
	if i, ok := r.classInfo[gwClass.Name]; ok {
		status.SupportedFeatures = i.SupportedFeatures
	}

	_, err := r.gwClassClient.UpdateStatus(&gwv1.GatewayClass{
		ObjectMeta: pluginsdk.CloneObjectMetaForStatus(gwClass.ObjectMeta),
		Status:     status,
	})
	if err != nil {
		return fmt.Errorf("error updating GatewayClass status: %w", err)
	}

	return nil
}

func (r *gatewayClassReconciler) createGatewayClasses() error {
	var errs []error
	for name, info := range r.classInfo {
		if err := r.createGatewayClass(name, info); err != nil {
			errs = append(errs, err)
			continue
		}
		logger.Info("created GatewayClass", "name", name)
	}
	return errors.Join(errs...)
}

func (r *gatewayClassReconciler) createGatewayClass(name string, info *deployer.GatewayClassInfo) error {
	gwc := r.gwClassClient.Get(name, metav1.NamespaceNone)
	if gwc != nil {
		// already exists, nothing to do
		return nil
	}

	gwc = &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: info.Annotations,
			Labels:      info.Labels,
		},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: gwv1.GatewayController(r.getControllerName(name)),
		},
	}
	if info.Description != "" {
		gwc.Spec.Description = ptr.To(info.Description)
	}
	if info.ParametersRef != nil {
		gwc.Spec.ParametersRef = info.ParametersRef
	}
	_, err := r.gwClassClient.Create(gwc)
	return err
}

func isOurGatewayClass(gwc *gwv1.GatewayClass, ourControllers sets.Set[string]) bool {
	return ourControllers.Has(string(gwc.Spec.ControllerName))
}

func (r *gatewayClassReconciler) getControllerName(gwc string) string {
	controllerName := r.defaultControllerName
	info, ok := r.classInfo[gwc]
	if ok && info.ControllerName != "" {
		controllerName = info.ControllerName
	}
	return controllerName
}
