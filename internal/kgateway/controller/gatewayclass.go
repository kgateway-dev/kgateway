package controller

import (
	"context"
	"fmt"

	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

var _ manager.LeaderElectionRunnable = (*gatewayClassReconciler)(nil)

type gatewayClassReconciler struct {
	classInfo     map[string]*deployer.GatewayClassInfo
	gwClassClient kclient.Client[*gwv1.GatewayClass]
	queue         controllers.Queue
}

func newGatewayClassReconciler(
	cfg GatewayConfig,
	classInfo map[string]*deployer.GatewayClassInfo,
) *gatewayClassReconciler {
	filter := kclient.Filter{ObjectFilter: cfg.Client.ObjectFilter()}
	r := &gatewayClassReconciler{
		classInfo:     classInfo,
		gwClassClient: kclient.NewFilteredDelayed[*gwv1.GatewayClass](cfg.Client, gvr.GatewayClass_v1, filter),
	}
	r.queue = controllers.NewQueue("GatewayClassController", controllers.WithReconciler(r.Reconcile), controllers.WithMaxAttempts(10))

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
			default:
				// no-op
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
	// Wait for all caches to sync
	kube.WaitForCacheSync("GatewayClassController", ctx.Done(), r.gwClassClient.HasSynced)
	r.queue.Run(ctx.Done())

	// Shutdown all the clients
	controllers.ShutdownAll(r.gwClassClient)
	return nil
}

func (r *gatewayClassReconciler) Reconcile(req types.NamespacedName) (rErr error) {
	finishMetrics := collectReconciliationMetrics("gatewayclass", req)
	defer func() {
		finishMetrics(rErr)
	}()

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

func isOurGatewayClass(gwc *gwv1.GatewayClass, ourControllers sets.Set[string]) bool {
	return ourControllers.Has(string(gwc.Spec.ControllerName))
}
