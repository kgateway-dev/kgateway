package endpointpicker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	infextv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func buildRegisterCallback(
	ctx context.Context,
	commonCol *common.CommonCollections,
	bcol krt.Collection[ir.BackendObjectIR],
) func() {
	return func() {
		bcol.Register(func(o krt.Event[ir.BackendObjectIR]) {
			if o.Event == controllers.EventDelete {
				return
			}

			in := o.Latest()
			ir, ok := in.ObjIr.(*inferencePool)
			if !ok {
				return
			}

			poolNsName := types.NamespacedName{
				Name:      in.ObjectSource.Name,
				Namespace: in.ObjectSource.Namespace,
			}
			pool := new(infextv1a2.InferencePool)

			cli := commonCol.CrudClient
			err := retry.Do(
				func() error {
					// Get the InferencePool resource.
					if err := cli.Get(ctx, poolNsName, pool); err != nil {
						return err
					}

					irRoutes := commonCol.Routes.ListHTTPRoutesInNamespace(poolNsName.Namespace)

					// Check if any HTTPRoute references this InferencePool.
					gtwName := ir.referencedGateway(ctx, commonCol, irRoutes, poolNsName)
					if gtwName == "" {
						// If needed, remove the Kgateway-managed gateway from the InferencePool status.
						if err := removeGatewayParentRef(ctx, cli, pool, commonCol.GatewayIndex); err != nil {
							return err
						}
						return nil
					}

					// If needed, add kgtw's controller ParentRef to the InferencePool status.
					addGatewayParentRef(&pool.Status, gtwName)

					pIdx := findGatewayParentRef(&pool.Status, gtwName)
					if pIdx == -1 {
						logger.Debug(
							"inferencepool not managed by kgateway, bypassing status update",
							"namespace",
							poolNsName.Namespace,
							"name",
							poolNsName.Name,
						)
						return nil
					}

					// Build the InferencePool Accepted status condition.
					newAcceptedCond := buildAcceptedCondition(pool.Generation, commonCol.ControllerName)

					// Check if the Accepted condition already exists and is up-to-date.
					existingAcceptedCond := meta.FindStatusCondition(pool.Status.Parents[pIdx].Conditions, string(infextv1a2.InferencePoolConditionAccepted))
					ackCondChanged := false
					switch {
					case existingAcceptedCond == nil:
						// If the condition doesn't exist, add it.
						ackCondChanged = meta.SetStatusCondition(&pool.Status.Parents[pIdx].Conditions, newAcceptedCond)
					default:
						// If the condition exists, check if it needs to be updated.
						statusEq := existingAcceptedCond.Status == newAcceptedCond.Status
						reasonEq := existingAcceptedCond.Reason == newAcceptedCond.Reason
						messageEq := existingAcceptedCond.Message == newAcceptedCond.Message
						genEq := existingAcceptedCond.ObservedGeneration == newAcceptedCond.ObservedGeneration
						if !statusEq || !reasonEq || !messageEq || !genEq {
							ackCondChanged = meta.SetStatusCondition(&pool.Status.Parents[pIdx].Conditions, newAcceptedCond)
						}
					}

					// Build the InferencePool ResolvedRefs status condition.
					newRRCond := buildResolvedRefsCondition(pool.Generation, ir.errors)

					// Check if the Accepted condition already exists and is up-to-date.
					existingRRCond := meta.FindStatusCondition(pool.Status.Parents[pIdx].Conditions, string(infextv1a2.InferencePoolConditionResolvedRefs))
					rrCondChanged := false
					switch {
					case existingRRCond == nil:
						// If the condition doesn't exist, add it.
						rrCondChanged = meta.SetStatusCondition(&pool.Status.Parents[pIdx].Conditions, newRRCond)
					default:
						// If the condition exists, check if it needs to be updated.
						statusEq := existingRRCond.Status == newRRCond.Status
						reasonEq := existingRRCond.Reason == newRRCond.Reason
						messageEq := existingRRCond.Message == newRRCond.Message
						genEq := existingRRCond.ObservedGeneration == newRRCond.ObservedGeneration
						if !statusEq || !reasonEq || !messageEq || !genEq {
							rrCondChanged = meta.SetStatusCondition(&pool.Status.Parents[pIdx].Conditions, newRRCond)
						}
					}

					if ackCondChanged || rrCondChanged {
						if err := cli.Status().Patch(ctx, pool, client.Merge); err != nil {
							return err
						}
						logger.Info(
							"patched inferencepool status",
							"name", poolNsName.String(),
							"namespace", poolNsName.Namespace,
						)
					}

					return nil
				},
				retry.Attempts(5),
				retry.Delay(100*time.Millisecond),
				retry.DelayType(retry.BackOffDelay),
			)
			if err != nil {
				logger.Error(
					"all attempts failed for patching resource status",
					"inferencepool",
					poolNsName.String(),
					"error",
					err,
				)
			}
		})
	}
}

// referencedGateway returns the gateway name if any HTTPRoute in irRoutes references
// the given InferencePool and is managed by kgateway.
func (p *inferencePool) referencedGateway(
	ctx context.Context,
	commonCol *common.CommonCollections,
	irRoutes []ir.HttpRouteIR,
	poolNN types.NamespacedName,
) string {
	if len(irRoutes) == 0 {
		logger.Debug("no IR HTTPRoutes found")
		return ""
	}

	for _, irRoute := range irRoutes {
		route, ok := irRoute.SourceObject.(*gwv1.HTTPRoute)
		if !ok {
			p.errors = append(p.errors, fmt.Errorf("error casting IR Route to HTTPRoute"))
			logger.Error("error casting IR Route to HTTPRoute", "namespace", irRoute.Namespace, "name", irRoute.Name)
			return ""
		}
		backendMatches := false
		for _, rule := range route.Spec.Rules {
			for _, ref := range rule.BackendRefs {
				if ref.Group != nil &&
					*ref.Group == gwv1.Group(infextv1a2.GroupVersion.Group) &&
					ref.Kind != nil &&
					*ref.Kind == wellknown.InferencePoolKind &&
					ref.Name == gwv1.ObjectName(poolNN.Name) {
					backendMatches = true
					break
				}
			}
			if backendMatches {
				break
			}
		}
		if !backendMatches {
			continue
		}

		// Check status.ParentRefs for the kgtw controllerName.
		// TODO [danehans]: Support cross-namespace references https://github.com/kgateway-dev/kgateway/issues/11370
		if backendMatches {
			for _, parent := range route.Status.Parents {
				if parent.ControllerName == gwv1.GatewayController(commonCol.ControllerName) {
					return string(parent.ParentRef.Name)
				}
			}
		}
	}

	return ""
}

// removeGatewayParentRef removes any ParentStatus for the given pool if the status
// ParentRef.Name equals controllerName and patches the InferencePool status if anything changed.
func removeGatewayParentRef(
	ctx context.Context,
	cli client.Client,
	pool *infextv1a2.InferencePool,
	gwIdx *krtcollections.GatewayIndex,
) error {
	if pool == nil {
		return fmt.Errorf("InferencePool is nil")
	}

	if pool.Status.Parents == nil || len(pool.Status.Parents) == 0 {
		// Nothing to do if the InferencePool status Parents is nil or empty.
		logger.Debug(
			"inferencepool status parents is nil or empty, nothing to remove",
			"namespace", pool.Namespace,
			"name", pool.Name,
		)
		return nil
	}

	gws := gwIdx.Gateways.List()
	if len(gws) == 0 {
		return fmt.Errorf("no Gateways found")
	}

	var updated []infextv1a2.PoolStatus
	exists := false
	for _, gw := range gws {
		if gw.Namespace != pool.Namespace {
			// TODO [danehans]: Support cross-namespace references by using commonCol RefGrants.
			continue
		}
		// Remove any ParentStatus whose GatewayRef.Name equals matchedGtw.
		for _, p := range pool.Status.Parents {
			if p.GatewayRef.Name == gw.Name {
				exists = true
				continue
			}
			updated = append(updated, p)
		}
	}

	// Nothing to do if we didn't remove a kgateway-managed GatewayRef.
	if !exists {
		return nil
	}

	pool.Status.Parents = updated
	if err := cli.Status().Patch(ctx, pool, client.Merge); err != nil {
		logger.Error(
			"failed to remove ParentRef from InferencePool status",
			"namespace", pool.Namespace,
			"name", pool.Name,
			"error", err,
		)
		return err
	}

	return nil
}

// addGatewayParentRef adds a GatewayRef to gtwName for the given pool's status
// if it does not already exist.
func addGatewayParentRef(status *infextv1a2.InferencePoolStatus, gtwName string) {
	if status == nil {
		status = &infextv1a2.InferencePoolStatus{
			Parents: []infextv1a2.PoolStatus{},
		}
	}

	for _, p := range status.Parents {
		if p.GatewayRef.Name == gtwName {
			// Nothing to do if the InferencePool already has this GatewayRef
			return
		}
	}

	status.Parents = append(status.Parents, infextv1a2.PoolStatus{
		GatewayRef: corev1.ObjectReference{
			Name: gtwName,
			Kind: "Gateway",
		},
	})
}

// findGatewayParentRef returns the PoolStatus index whose GatewayRef.Name
// equals controllerName, or -1 if no such parent exists.
func findGatewayParentRef(status *infextv1a2.InferencePoolStatus, gtwName string) int {
	if status == nil {
		status = &infextv1a2.InferencePoolStatus{
			Parents: []infextv1a2.PoolStatus{},
		}
	}

	for i, parent := range status.Parents {
		if parent.GatewayRef.Name == gtwName {
			return i
		}
	}

	return -1
}

func buildAcceptedCondition(gen int64, controllerName string) metav1.Condition {
	return metav1.Condition{
		Type:               string(infextv1a2.InferencePoolConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(infextv1a2.InferencePoolReasonAccepted),
		Message:            fmt.Sprintf("InferencePool has been accepted by controller %s", controllerName),
		ObservedGeneration: gen,
		LastTransitionTime: metav1.Now(),
	}
}

func buildResolvedRefsCondition(gen int64, errs []error) metav1.Condition {
	cond := metav1.Condition{
		Type:               string(infextv1a2.InferencePoolConditionResolvedRefs),
		ObservedGeneration: gen,
		LastTransitionTime: metav1.Now(),
	}

	if len(errs) == 0 {
		cond.Status = metav1.ConditionTrue
		cond.Reason = string(infextv1a2.InferencePoolReasonResolvedRefs)
		cond.Message = "All InferencePool references have been resolved"
		return cond
	}

	// Build a human-friendly prefix.
	var prefix string
	if len(errs) == 1 {
		prefix = "error:"
	} else {
		prefix = fmt.Sprintf("InferencePool has %d errors:", len(errs))
	}

	// Collect and semicolon-join all error messages.
	msgs := make([]string, 0, len(errs))
	for _, err := range errs {
		msgs = append(msgs, err.Error())
	}
	joined := strings.Join(msgs, "; ")

	cond.Status = metav1.ConditionFalse
	cond.Reason = string(infextv1a2.InferencePoolReasonInvalidExtensionRef)
	cond.Message = fmt.Sprintf("%s %s", prefix, joined)
	return cond
}
