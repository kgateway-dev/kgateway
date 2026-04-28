package proxy_syncer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	utilretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/metrics"
	plug "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	reportssdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

var _ manager.LeaderElectionRunnable = &StatusSyncer{}

// StatusSyncer runs only on the leader and syncs the status of resources.
type StatusSyncer struct {
	mgr            manager.Manager
	plugins        plug.Plugin
	controllerName string
	istioClient    apiclient.Client

	latestReportQueue              utils.AsyncQueue[reports.ReportMap]
	latestBackendPolicyReportQueue utils.AsyncQueue[reports.ReportMap]
	cacheSyncs                     []cache.InformerSynced

	customStatusSync func(ctx context.Context, rm reports.ReportMap)
}

func NewStatusSyncer(
	mgr manager.Manager,
	plugins plug.Plugin,
	controllerName string,
	client apiclient.Client,
	commonCols *collections.CommonCollections,
	reportQueue utils.AsyncQueue[reports.ReportMap],
	backendPolicyReportQueue utils.AsyncQueue[reports.ReportMap],
	cacheSyncs []cache.InformerSynced,
	opts ...StatusSyncerOption,
) *StatusSyncer {
	cfg := processStatusSyncerOptions(opts...)
	return &StatusSyncer{
		mgr:                            mgr,
		plugins:                        plugins,
		istioClient:                    client,
		controllerName:                 controllerName,
		latestReportQueue:              reportQueue,
		latestBackendPolicyReportQueue: backendPolicyReportQueue,
		cacheSyncs:                     cacheSyncs,
		customStatusSync:               cfg.CustomStatusSync,
	}
}

func (s *StatusSyncer) Start(ctx context.Context) error {
	logger.Info("starting Status Syncer", "controller", s.controllerName)

	// wait for krt collections to sync
	logger.Info("waiting for cache to sync")
	s.istioClient.WaitForCacheSync(
		"kube gw status syncer",
		ctx.Done(),
		s.cacheSyncs...,
	)

	// wait for ctrl-rtime caches to sync before accepting events
	if !s.mgr.GetCache().WaitForCacheSync(ctx) {
		return errors.New("kube gateway status syncer sync loop waiting for all caches to sync failed")
	}
	logger.Info("caches warm!")

	// caches are warm, now we can do registrations
	for _, regFunc := range s.plugins.ContributesLeaderAction {
		if regFunc != nil {
			regFunc()
		}
	}

	routeStatusLogger := logger.With("subcomponent", "routeStatusSyncer")
	listenerSetStatusLogger := logger.With("subcomponent", "listenerSetStatusSyncer")
	gatewayStatusLogger := logger.With("subcomponent", "gatewayStatusSyncer")
	go func() {
		for {
			latestReport, err := s.latestReportQueue.Dequeue(ctx)
			if err != nil {
				logger.Error("failed to dequeue gateway reports", "error", err)
				return
			}
			s.syncGatewayStatus(ctx, gatewayStatusLogger, latestReport)
			s.syncListenerSetStatus(ctx, listenerSetStatusLogger, latestReport)
			s.syncRouteStatus(ctx, routeStatusLogger, latestReport)
			s.syncPolicyStatus(ctx, latestReport)
			if s.customStatusSync != nil {
				s.customStatusSync(ctx, latestReport)
			}
		}
	}()
	go func() {
		for {
			latestReport, err := s.latestBackendPolicyReportQueue.Dequeue(ctx)
			if err != nil {
				logger.Error("failed to dequeue backend policy reports", "error", err)
				return
			}
			s.syncPolicyStatus(ctx, latestReport)
		}
	}()

	<-ctx.Done()
	return nil
}

func (s *StatusSyncer) syncRouteStatus(ctx context.Context, logger *slog.Logger, rm reports.ReportMap) {
	// Helper function to sync route status with retry
	syncStatusWithRetry := func(
		routeType string,
		routeKey client.ObjectKey,
		getRouteFunc func(context.Context, client.ObjectKey) (client.Object, error),
		statusUpdater func(route client.Object) (*gwv1.RouteStatus, error),
	) error {
		return retry.Do(
			func() (rErr error) {
				route, err := getRouteFunc(ctx, routeKey)
				if err != nil {
					if apierrors.IsNotFound(err) {
						// the route is not found, we can't report status on it
						// if it's recreated, we'll retranslate it anyway
						return nil
					}
					logger.Error("error getting route", "error", err, "resource_ref", routeKey, "route_type", routeType)
					return err
				}

				gatewayNames := []string{}

				switch r := route.(type) {
				case *gwv1.HTTPRoute:
					for _, parentRef := range r.Spec.ParentRefs {
						gatewayNames = append(gatewayNames, string(parentRef.Name))
					}
				case *gwv1a2.TCPRoute:
					for _, parentRef := range r.Spec.ParentRefs {
						gatewayNames = append(gatewayNames, string(parentRef.Name))
					}
				case *gwv1.TLSRoute:
					for _, parentRef := range r.Spec.ParentRefs {
						gatewayNames = append(gatewayNames, string(parentRef.Name))
					}
				case *gwv1a2.TLSRoute:
					for _, parentRef := range r.Spec.ParentRefs {
						gatewayNames = append(gatewayNames, string(parentRef.Name))
					}
				case *unstructured.Unstructured:
					if unstructuredTLSRoute := collections.ConvertUnstructuredTLSRouteToV1Alpha2ForStatus(r); unstructuredTLSRoute != nil {
						for _, parentRef := range unstructuredTLSRoute.Spec.ParentRefs {
							gatewayNames = append(gatewayNames, string(parentRef.Name))
						}
					}
				case *gwv1.GRPCRoute:
					for _, parentRef := range r.Spec.ParentRefs {
						gatewayNames = append(gatewayNames, string(parentRef.Name))
					}
				default:
					logger.Warn("unknown route type during status sync", "route_type",
						routeType, "resource_ref", client.ObjectKeyFromObject(route))
				}

				type finishMetricsErrors struct {
					finishFunc  func(error)
					statusError error
				}

				finishMetrics := make(map[string]finishMetricsErrors, len(gatewayNames))

				for _, gatewayName := range gatewayNames {
					finishMetrics[gatewayName] = finishMetricsErrors{
						finishFunc: CollectStatusSyncMetrics(StatusSyncMetricLabels{
							Name:      gatewayName,
							Namespace: routeKey.Namespace,
							Syncer:    "RouteStatusSyncer",
						}),
					}
				}

				defer func() {
					for _, gatewayName := range gatewayNames {
						metrics.EndResourceStatusSync(metrics.ResourceSyncDetails{
							Namespace:    routeKey.Namespace,
							Gateway:      gatewayName,
							ResourceType: routeType,
							ResourceName: routeKey.Name,
						})

						if finish, exists := finishMetrics[gatewayName]; exists {
							finish.finishFunc(errors.Join(rErr, finish.statusError))
						}
					}
				}()

				if status, err := statusUpdater(route); err != nil {
					logger.Debug("error updating status for route", "error", err, "resource_ref", routeKey, "route_type", routeType)
					return err
				} else if status != nil {
					// Update metrics status if the conditions indicate an error.
					for _, ps := range status.Parents {
						for _, cond := range ps.Conditions {
							if cond.Type == string(gwv1.RouteConditionPartiallyInvalid) && cond.Status == metav1.ConditionTrue {
								if finish, exists := finishMetrics[string(ps.ParentRef.Name)]; exists {
									finishMetrics[string(ps.ParentRef.Name)] = finishMetricsErrors{
										finishFunc:  finish.finishFunc,
										statusError: fmt.Errorf("partially invalid route condition"),
									}

									break
								}
							}

							if cond.Type != string(gwv1.RouteConditionAccepted) {
								continue
							}

							if cond.Reason != string(gwv1.RouteReasonAccepted) &&
								cond.Reason != string(gwv1.RouteReasonPending) {
								if finish, exists := finishMetrics[string(ps.ParentRef.Name)]; exists {
									finishMetrics[string(ps.ParentRef.Name)] = finishMetricsErrors{
										finishFunc:  finish.finishFunc,
										statusError: fmt.Errorf("invalid route condition"),
									}

									break
								}
							}
						}
					}
				}

				return nil
			},
			retry.Attempts(5),
			retry.Delay(100*time.Millisecond),
			retry.DelayType(retry.BackOffDelay),
		)
	}

	// Helper function to build route status and update if needed
	buildAndUpdateStatus := func(route client.Object, routeType string) (*gwv1.RouteStatus, error) {
		var status *gwv1.RouteStatus
		switch r := route.(type) {
		case *gwv1.HTTPRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil, nil
			}
			r.Status.RouteStatus = *status
		case *gwv1a2.TCPRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil, nil
			}
			r.Status.RouteStatus = *status
		case *gwv1.TLSRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil, nil
			}
			r.Status.RouteStatus = *status
		case *gwv1a2.TLSRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil, nil
			}
			r.Status.RouteStatus = *status
		case *unstructured.Unstructured:
			unstructuredTLSRoute := collections.ConvertUnstructuredTLSRouteToV1Alpha2ForStatus(r)
			if unstructuredTLSRoute == nil {
				return nil, nil
			}
			status = rm.BuildRouteStatus(ctx, unstructuredTLSRoute, s.controllerName)
			if status == nil || isRouteStatusEqual(&unstructuredTLSRoute.Status.RouteStatus, status) {
				return nil, nil
			}
			return status, updateUnstructuredTLSRouteStatus(ctx, s.mgr.GetClient().Status(), r, *status)
		case *gwv1.GRPCRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil, nil
			}
			r.Status.RouteStatus = *status
		default:
			logger.Warn("unsupported route type", "route_type", routeType, "resource_ref", client.ObjectKeyFromObject(route))
			return nil, nil
		}

		// Update the status
		return status, s.mgr.GetClient().Status().Update(ctx, route)
	}

	// Sync HTTPRoute statuses
	for rnn := range rm.HTTPRoutes {
		err := syncStatusWithRetry(
			wellknown.HTTPRouteKind,
			rnn,
			func(ctx context.Context, routeKey client.ObjectKey) (client.Object, error) {
				route := new(gwv1.HTTPRoute)
				return route, s.mgr.GetClient().Get(ctx, routeKey, route)
			},
			func(route client.Object) (*gwv1.RouteStatus, error) {
				return buildAndUpdateStatus(route, wellknown.HTTPRouteKind)
			},
		)
		if err != nil {
			logger.Error("all attempts failed at updating HTTPRoute status", "error", err, "route", rnn)
		}
	}

	// Sync TCPRoute statuses
	for rnn := range rm.TCPRoutes {
		err := syncStatusWithRetry(wellknown.TCPRouteKind, rnn,
			func(ctx context.Context, routeKey client.ObjectKey) (client.Object, error) {
				route := new(gwv1a2.TCPRoute)
				return route, s.mgr.GetClient().Get(ctx, routeKey, route)
			},
			func(route client.Object) (*gwv1.RouteStatus, error) {
				return buildAndUpdateStatus(route, wellknown.TCPRouteKind)
			})
		if err != nil {
			logger.Error("all attempts failed at updating TCPRoute status", "error", err, "route", rnn)
		}
	}

	// Sync TLSRoute statuses
	for rnn := range rm.TLSRoutes {
		err := syncStatusWithRetry(wellknown.TLSRouteKind, rnn,
			func(ctx context.Context, routeKey client.ObjectKey) (client.Object, error) {
				return getTLSRouteForStatus(ctx, s.mgr.GetClient(), routeKey)
			},
			func(route client.Object) (*gwv1.RouteStatus, error) {
				return buildAndUpdateStatus(route, wellknown.TLSRouteKind)
			})
		if err != nil {
			logger.Error("all attempts failed at updating TLSRoute status", "error", err, "route", rnn)
		}
	}

	// Sync GRPCRoute statuses
	for rnn := range rm.GRPCRoutes {
		err := syncStatusWithRetry(wellknown.GRPCRouteKind, rnn,
			func(ctx context.Context, routeKey client.ObjectKey) (client.Object, error) {
				route := new(gwv1.GRPCRoute)
				return route, s.mgr.GetClient().Get(ctx, routeKey, route)
			},
			func(route client.Object) (*gwv1.RouteStatus, error) {
				return buildAndUpdateStatus(route, wellknown.GRPCRouteKind)
			})
		if err != nil {
			logger.Error("all attempts failed at updating GRPCRoute status", "error", err, "route", rnn)
		}
	}
}

type objectGetter interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
}

type statusWriter interface {
	Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error
}

func getTLSRouteForStatus(ctx context.Context, kubeClient objectGetter, key client.ObjectKey) (client.Object, error) {
	promotedTLSRoute := &gwv1.TLSRoute{}
	if err := kubeClient.Get(ctx, key, promotedTLSRoute); err == nil {
		return promotedTLSRoute, nil
	} else if !shouldFallbackTLSRouteLookup(err) {
		return nil, err
	}

	v1alpha3TLSRouteRaw := &unstructured.Unstructured{}
	v1alpha3TLSRouteRaw.SetGroupVersionKind(wellknown.TLSRouteV1Alpha3GVK)
	if err := kubeClient.Get(ctx, key, v1alpha3TLSRouteRaw); err == nil {
		return v1alpha3TLSRouteRaw, nil
	} else if !shouldFallbackTLSRouteLookup(err) {
		return nil, err
	}

	v1alpha2TLSRoute := &gwv1a2.TLSRoute{}
	if err := kubeClient.Get(ctx, key, v1alpha2TLSRoute); err != nil {
		return nil, err
	}
	return v1alpha2TLSRoute, nil
}

func shouldFallbackTLSRouteLookup(err error) bool {
	return apierrors.IsNotFound(err) || apimeta.IsNoMatchError(err)
}

func updateUnstructuredTLSRouteStatus(ctx context.Context, writer statusWriter, route *unstructured.Unstructured, status gwv1.RouteStatus) error {
	statusMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&gwv1a2.TLSRouteStatus{
		RouteStatus: status,
	})
	if err != nil {
		return err
	}
	route.Object["status"] = statusMap
	return writer.Update(ctx, route)
}

// syncGatewayStatus will build and update status for all Gateways in a reportMap
func (s *StatusSyncer) syncGatewayStatus(ctx context.Context, logger *slog.Logger, rm reports.ReportMap) {
	for gwnn := range rm.Gateways {
		finishMetrics := CollectStatusSyncMetrics(StatusSyncMetricLabels{
			Name:      gwnn.Name,
			Namespace: gwnn.Namespace,
			Syncer:    "GatewayStatusSyncer",
		})

		var statusErr error

		err := utilretry.RetryOnConflict(utilretry.DefaultRetry, func() error {
			// Fetch the latest Gateway
			var gw gwv1.Gateway
			if err := s.mgr.GetClient().Get(ctx, gwnn, &gw); err != nil {
				logger.Info("error getting gateway", "error", err, "gateway", gwnn.String())
				return err
			}

			// Check the controller name of the gateway class to avoid syncing status for non-envoy controllers
			gwClass := gwv1.GatewayClass{}
			err := s.mgr.GetClient().Get(ctx, types.NamespacedName{
				Name: string(gw.Spec.GatewayClassName),
			}, &gwClass)
			if err != nil {
				if apierrors.IsNotFound(err) {
					logger.Debug("gateway class not found, skipping", "gateway", gwnn.String(), "gatewayClassName", gw.Spec.GatewayClassName)
					return nil
				}
				logger.Error("error getting gateway class", "error", err, "gateway", gwnn.String(), "gatewayClassName", gw.Spec.GatewayClassName)
				return err
			}

			if string(gwClass.Spec.ControllerName) != s.controllerName {
				logger.Debug("skipping status sync for non-kgateway controller", "gateway", gwnn.String(), "controllerName", gwClass.Spec.ControllerName, "gatewayClassName", gw.Spec.GatewayClassName)
				return nil
			}

			// Build the desired status
			newStatus := rm.BuildGWStatus(ctx, gw, nil)
			if newStatus == nil {
				logger.Debug("new status is nil; skipping status update", "gateway", gwnn.String())
				return nil
			}

			// Skip if status hasn’t changed (ignoring Addresses)
			old := gw.Status
			old.Addresses = nil
			if isGatewayStatusEqual(&old, newStatus) {
				logger.Debug("gateway status is equal; skipping status update", "gateway", gwnn.String())
				return nil
			}

			// Apply the status update
			gw.Status = *newStatus
			if err := s.mgr.GetClient().Status().Update(ctx, &gw); err != nil {
				if !apierrors.IsConflict(err) {
					logger.Error("error updating gateway status", "error", err, "gateway", gwnn.String())
				}
				return err
			}
			logger.Info("updated gateway status", "gateway", gwnn.String())

			for _, cond := range gw.Status.Conditions {
				if cond.Type != string(gwv1.GatewayConditionAccepted) &&
					cond.Type != string(gwv1.GatewayConditionProgrammed) {
					continue
				}

				if cond.Reason != string(gwv1.GatewayReasonAccepted) &&
					cond.Reason != string(gwv1.GatewayReasonProgrammed) &&
					cond.Reason != string(gwv1.GatewayReasonPending) {
					logger.Debug("invalid status condition reason", "reason", cond.Reason, "gateway", gwnn.String())

					statusErr = fmt.Errorf("invalid gateway condition")

					break
				}
			}

			return nil
		})
		if err != nil {
			logger.Error("failed to update gateway status after retries", "error", err, "gateway", gwnn.String())
		}

		// Record metrics for this gateway
		metrics.EndResourceStatusSync(metrics.ResourceSyncDetails{
			Namespace:    gwnn.Namespace,
			Gateway:      gwnn.Name,
			ResourceType: wellknown.GatewayKind,
			ResourceName: gwnn.Name,
		})

		finishMetrics(errors.Join(err, statusErr))
	}

	logger.Debug("synced gw status for gateways", "count", len(rm.Gateways))
}

// syncListenerSetStatus will build and update status for all Listener Sets in a reportMap
func (s *StatusSyncer) syncListenerSetStatus(ctx context.Context, logger *slog.Logger, rm reports.ReportMap) {
	// TODO: retry within loop per LS rather than as a full block
	err := retry.Do(func() (rErr error) {
		for gvk, listenerSetsForGVK := range rm.ListenerSets {
			for lsnn := range listenerSetsForGVK {
				ls, legacyListenerSet, err := s.getListenerSetForStatus(ctx, lsnn, gvk)
				if err != nil {
					logger.Info("error getting ls", "error", err.Error())
					return err
				}

				var statusErr error

				finishMetrics := CollectStatusSyncMetrics(StatusSyncMetricLabels{
					Name:      string(ls.Spec.ParentRef.Name),
					Namespace: lsnn.Namespace,
					Syncer:    "ListenerSetStatusSyncer",
				})
				defer func() {
					finishMetrics(errors.Join(rErr, statusErr))
				}()

				lsStatus := ls.Status
				if status := rm.BuildListenerSetStatus(ctx, ls); status != nil {
					if !isListenerSetStatusEqual(&lsStatus, status) {
						ls.Status = *status
						if err := s.patchListenerSetStatus(ctx, &ls, legacyListenerSet); err != nil {
							if !apierrors.IsConflict(err) {
								logger.Error("error patching listener set status", "error", err, "gateway", lsnn.String())
							}
							return err
						}
						logger.Info("patched ls status", "listenerset", lsnn.String(), "gvk", gvk.String())

						for _, cond := range status.Conditions {
							if cond.Type != string(gwv1.ListenerSetConditionAccepted) &&
								cond.Type != string(gwv1.ListenerSetConditionProgrammed) {
								continue
							}

							if cond.Reason != string(gwv1.ListenerSetReasonAccepted) &&
								cond.Reason != string(gwv1.ListenerSetReasonProgrammed) &&
								cond.Reason != string(gwv1.ListenerSetReasonPending) {
								statusErr = fmt.Errorf("invalid listener condition")

								break
							}
						}
					} else {
						logger.Debug("skipping k8s ls status update, status equal", "listenerset", lsnn.String(), "gvk", gvk.String())
					}

					metrics.EndResourceStatusSync(metrics.ResourceSyncDetails{
						Namespace: ls.Namespace,
						Gateway:   string(ls.Spec.ParentRef.Name),
						// TODO: Rename the legacy "XListenerSet" metrics label to "ListenerSet" in a
						// follow-up cleanup so dashboards, tests, and emitters can be updated together.
						ResourceType: "XListenerSet",
						ResourceName: ls.Name,
					})
				}
			}
		}
		return nil
	},
		retry.Attempts(5),
		retry.Delay(100*time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		logger.Error("all attempts failed at updating listener set statuses", "error", err)
	}
	logger.Debug("synced listener sets status for listener set", "count", len(rm.ListenerSets))
}

func (s *StatusSyncer) getListenerSetForStatus(
	ctx context.Context,
	key types.NamespacedName,
	gvk schema.GroupVersionKind,
) (gwv1.ListenerSet, *unstructured.Unstructured, error) {
	if gvk == wellknown.XListenerSetGVK {
		legacyListenerSet := &unstructured.Unstructured{}
		legacyListenerSet.SetGroupVersionKind(gvk)
		if err := s.mgr.GetClient().Get(ctx, key, legacyListenerSet); err != nil {
			return gwv1.ListenerSet{}, nil, err
		}

		ls := gwv1.ListenerSet{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(legacyListenerSet.UnstructuredContent(), &ls); err != nil {
			return gwv1.ListenerSet{}, nil, err
		}
		ls.SetGroupVersionKind(gvk)
		return ls, legacyListenerSet, nil
	}

	ls := gwv1.ListenerSet{}
	if err := s.mgr.GetClient().Get(ctx, key, &ls); err != nil {
		return gwv1.ListenerSet{}, nil, err
	}
	if ls.GroupVersionKind().Empty() {
		ls.SetGroupVersionKind(wellknown.ListenerSetGVK)
	}
	return ls, nil, nil
}

func (s *StatusSyncer) patchListenerSetStatus(
	ctx context.Context,
	ls *gwv1.ListenerSet,
	legacyListenerSet *unstructured.Unstructured,
) error {
	if legacyListenerSet == nil {
		return s.mgr.GetClient().Status().Patch(ctx, ls, client.Merge)
	}

	statusMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&ls.Status)
	if err != nil {
		return err
	}
	legacyListenerSet.Object["status"] = statusMap
	return s.mgr.GetClient().Status().Patch(ctx, legacyListenerSet, client.Merge)
}

func (s *StatusSyncer) syncPolicyStatus(ctx context.Context, rm reports.ReportMap) {
	// Sync Policy statuses
	for key := range rm.Policies {
		gk := schema.GroupKind{Group: key.Group, Kind: key.Kind}
		nsName := types.NamespacedName{Namespace: key.Namespace, Name: key.Name}

		plugin, ok := s.plugins.ContributesPolicies[gk]
		if !ok {
			logger.Error("policy plugin not registered for policy", "group_kind", gk, "resource", nsName)
			continue
		}
		if plugin.GetPolicyStatus == nil {
			logger.Debug("GetPolicyStatus handler not registered for policy", "group_kind", gk, "resource", nsName) //nolint:sloglint // ignore PascalCase at start
			continue
		}
		if plugin.PatchPolicyStatus == nil {
			logger.Debug("PatchPolicyStatus handler not registered for policy", "group_kind", gk, "resource", nsName) //nolint:sloglint // ignore PascalCase at start
			continue
		}
		currentStatus, err := plugin.GetPolicyStatus(ctx, nsName)
		if err != nil {
			logger.Error("error getting policy status", "error", err, "resource_ref", nsName)
			continue
		}
		status := buildPolicyStatus(ctx, rm, plugin, key, s.controllerName, currentStatus)
		if status == nil {
			continue
		}

		var statusErr error

		if plugin.BuildPolicyStatus == nil {
			for _, ancestor := range status.Ancestors {
				for _, cond := range ancestor.Conditions {
					if cond.Type != string(shared.PolicyConditionAccepted) {
						continue
					}

					if cond.Reason != string(shared.PolicyReasonValid) &&
						cond.Reason != string(shared.PolicyReasonPending) {
						statusErr = fmt.Errorf("invalid policy condition")

						break
					}
				}

				if statusErr != nil {
					break
				}
			}
		}

		finishMetrics := CollectStatusSyncMetrics(StatusSyncMetricLabels{
			Name:      gk.Kind,
			Namespace: nsName.Namespace,
			Syncer:    "PolicyStatusSyncer",
		})

		err = retry.Do(
			func() error {
				return plugin.PatchPolicyStatus(ctx, nsName, *status)
			},
			retry.Attempts(5),
			retry.Delay(100*time.Millisecond),
			retry.DelayType(retry.BackOffDelay),
		)
		if err != nil {
			logger.Error("error updating policy status", "error", err, "group_kind", gk, "resource_ref", nsName)
			finishMetrics(errors.Join(err, statusErr))
			continue
		}

		metrics.EndResourceStatusSync(metrics.ResourceSyncDetails{
			Namespace:    nsName.Namespace,
			Gateway:      "",
			ResourceType: gk.Kind,
			ResourceName: nsName.Name,
		})

		finishMetrics(statusErr)
	}
}

func buildPolicyStatus(
	ctx context.Context,
	rm reports.ReportMap,
	plugin plug.PolicyPlugin,
	key reportssdk.PolicyKey,
	controllerName string,
	currentStatus gwv1.PolicyStatus,
) *gwv1.PolicyStatus {
	if plugin.BuildPolicyStatus != nil {
		return plugin.BuildPolicyStatus(ctx, rm, key, controllerName, currentStatus)
	}

	return rm.BuildPolicyStatus(ctx, key, controllerName, currentStatus)
}

// NeedLeaderElection returns true to ensure that the StatusSyncer runs only on the leader
func (r *StatusSyncer) NeedLeaderElection() bool {
	return true
}

var opts = cmp.Options{
	cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
	cmpopts.IgnoreMapEntries(func(k string, _ any) bool {
		return k == "lastTransitionTime"
	}),
}

func isGatewayStatusEqual(objA, objB *gwv1.GatewayStatus) bool {
	return cmp.Equal(objA, objB, opts)
}

func isListenerSetStatusEqual(objA, objB *gwv1.ListenerSetStatus) bool {
	return cmp.Equal(objA, objB, opts)
}

// isRouteStatusEqual compares two RouteStatus objects directly
func isRouteStatusEqual(objA, objB *gwv1.RouteStatus) bool {
	return cmp.Equal(objA, objB, opts)
}
