package plugins

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/agentgatewaysyncer/status"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

const (
	// Default names for the InferencePool status object.
	defaultInfPoolStatusKind = "Status"
	defaultInfPoolStatusName = "default"
	// Error messages for InferencePool validation.
	errInvalidGroupFormat     = "invalid extensionRef: only core API group supported, got %q"
	errInvalidKindFormat      = "invalid extensionRef: Kind %q is not supported (only Service)"
	errInvalidOneTargetPort   = "invalid InferencePool: must have exactly one target port"
	errPortRequired           = "invalid extensionRef port must be specified"
	errServiceNotFoundFormat  = "invalid extensionRef: Service %s/%s not found"
	errExternalNameNotAllowed = "invalid extensionRef: must use any Service type other than ExternalName"
	errTCPPortNotFoundFormat  = "TCP port %d not found on Service %s/%s"
)

// NewInferencePlugin creates a new InferencePool policy plugin
func NewInferencePlugin(agw *AgwCollections) AgwPlugin {
	policyCol := krt.NewManyCollection(agw.InferencePools, func(krtctx krt.HandlerContext, infPool *inf.InferencePool) []AgwPolicy {
		return translatePoliciesForInferencePool(infPool)
	})
	statusCol := newInferencePoolStatusCollection(agw)
	return AgwPlugin{
		ContributesPolicies: map[schema.GroupKind]PolicyPlugin{
			wellknown.InferencePoolGVK.GroupKind(): {
				Policies: policyCol,
			},
		},
		RegisterStatuses: func(sc *status.StatusCollections) {
			status.RegisterStatus(sc, statusCol, func(pool *inf.InferencePool) inf.InferencePoolStatus {
				return pool.Status
			})
		},
		ExtraHasSynced: func() bool {
			return policyCol.HasSynced() && statusCol.HasSynced()
		},
	}
}

// translatePoliciesForInferencePool generates policies for a single inference pool
func translatePoliciesForInferencePool(pool *inf.InferencePool) []AgwPolicy {
	var infPolicies []AgwPolicy

	// 'service/{namespace}/{hostname}:{port}'
	hostname := kubeutils.GetInferenceServiceHostname(pool.Name, pool.Namespace)

	epr := pool.Spec.EndpointPickerRef
	if epr.Group != nil && *epr.Group != "" {
		logger.Warn("inference pool endpoint picker ref has non-empty group, skipping", "pool", pool.Name, "group", *epr.Group)
		return nil
	}

	if epr.Kind != wellknown.ServiceKind {
		logger.Warn("inference pool extension ref is not a Service, skipping", "pool", pool.Name, "kind", epr.Kind)
		return nil
	}

	if epr.Port == nil {
		logger.Warn("inference pool extension ref port must be specified, skipping", "pool", pool.Name, "kind", epr.Kind)
		return nil
	}

	eppPort := epr.Port.Number

	eppSvc := kubeutils.GetServiceHostname(string(epr.Name), pool.Namespace)

	failureMode := api.BackendPolicySpec_InferenceRouting_FAIL_CLOSED
	if epr.FailureMode == inf.EndpointPickerFailOpen {
		failureMode = api.BackendPolicySpec_InferenceRouting_FAIL_OPEN
	}

	// Create the inference routing policy
	inferencePolicy := &api.Policy{
		Key:    pool.Namespace + "/" + pool.Name + ":inference",
		Name:   TypedResourceName(wellknown.InferencePoolGVK.Kind, pool),
		Target: &api.PolicyTarget{Kind: utils.ServiceTargetWithHostname(pool.Namespace, hostname, nil)},
		Kind: &api.Policy_Backend{
			Backend: &api.BackendPolicySpec{
				Kind: &api.BackendPolicySpec_InferenceRouting_{
					InferenceRouting: &api.BackendPolicySpec_InferenceRouting{
						EndpointPicker: &api.BackendReference{
							Kind: &api.BackendReference_Service_{
								Service: &api.BackendReference_Service{
									Hostname:  eppSvc,
									Namespace: pool.Namespace,
								},
							},
							Port: uint32(eppPort), //nolint:gosec // G115: eppPort is derived from validated port numbers
						},
						FailureMode: failureMode,
					},
				},
			},
		},
	}
	infPolicies = append(infPolicies, AgwPolicy{Policy: inferencePolicy})

	// Create the TLS policy for the endpoint picker
	// TODO: we would want some way if they explicitly set a BackendTLSPolicy for the EPP to respect that
	inferencePolicyTLS := &api.Policy{
		Key:    pool.Namespace + "/" + pool.Name + ":inferencetls",
		Name:   TypedResourceName(wellknown.InferencePoolGVK.Kind, pool),
		Target: &api.PolicyTarget{Kind: utils.ServiceTargetWithHostname(pool.Namespace, eppSvc, ptr.Of(strconv.Itoa(int(eppPort))))},
		Kind: &api.Policy_Backend{
			Backend: &api.BackendPolicySpec{
				Kind: &api.BackendPolicySpec_BackendTls{
					BackendTls: &api.BackendPolicySpec_BackendTLS{
						// The spec mandates this :vomit:
						Verification: api.BackendPolicySpec_BackendTLS_INSECURE_ALL,
					},
				},
			},
		},
	}
	infPolicies = append(infPolicies, AgwPolicy{Policy: inferencePolicyTLS})

	logger.Debug("generated inference pool policies",
		"pool", pool.Name,
		"namespace", pool.Namespace,
		"inference_policy", inferencePolicy.Name,
		"tls_policy", inferencePolicyTLS.Name)

	return infPolicies
}

func newInferencePoolStatusCollection(agw *AgwCollections) krt.StatusCollection[*inf.InferencePool, inf.InferencePoolStatus] {
	return krt.NewCollection(agw.InferencePools, func(ctx krt.HandlerContext, pool *inf.InferencePool) *krt.ObjectWithStatus[*inf.InferencePool, inf.InferencePoolStatus] {
		status := buildInferencePoolStatus(ctx, agw, pool)
		if status == nil {
			return nil
		}
		return &krt.ObjectWithStatus[*inf.InferencePool, inf.InferencePoolStatus]{
			Obj:    pool,
			Status: *status,
		}
	}, agw.KrtOpts.ToOptions("InferencePoolStatus")...)
}

func buildInferencePoolStatus(
	ctx krt.HandlerContext,
	agw *AgwCollections,
	pool *inf.InferencePool,
) *inf.InferencePoolStatus {
	errs := validateInferencePool(ctx, agw, pool)

	active := map[types.NamespacedName]struct{}{}
	type acceptanceState struct {
		anyAccepted bool
		anyRejected bool
	}
	acceptedByParent := map[types.NamespacedName]acceptanceState{}
	routes := krt.Fetch(ctx, agw.HTTPRoutes, krt.FilterIndex(agw.HTTPRoutesByNamespace, pool.Namespace))
	for _, rt := range routes {
		if !routeUsesPool(rt, pool.Namespace, pool.Name) {
			continue
		}

		parents := rt.Status.Parents
		useStatusParents := len(parents) != 0
		if !useStatusParents {
			parents = nil
			for _, pr := range rt.Spec.ParentRefs {
				parents = append(parents, gwv1.RouteParentStatus{ParentRef: pr})
			}
		}

		for _, ps := range parents {
			if ps.ControllerName != "" && string(ps.ControllerName) != agw.ControllerName {
				continue
			}
			pr := ps.ParentRef
			if pr.Group != nil && *pr.Group != gwv1.GroupName {
				continue
			}
			if pr.Kind != nil && string(*pr.Kind) != wellknown.GatewayKind {
				continue
			}
			ns := rt.Namespace
			if pr.Namespace != nil {
				ns = string(*pr.Namespace)
			}
			gwKey := types.NamespacedName{Namespace: ns, Name: string(pr.Name)}
			active[gwKey] = struct{}{}
			state := acceptedByParent[gwKey]
			if useStatusParents {
				cond := meta.FindStatusCondition(ps.Conditions, string(gwv1.RouteConditionAccepted))
				if cond != nil && cond.Status == metav1.ConditionFalse {
					state.anyRejected = true
				} else {
					// Treat unknown/missing Accepted as accepted to match legacy behavior.
					state.anyAccepted = true
				}
			} else {
				// No status parents: default to accepted for legacy behavior.
				state.anyAccepted = true
			}
			acceptedByParent[gwKey] = state
		}
	}

	var parents []inf.ParentStatus
	sortedGateways := make([]types.NamespacedName, 0, len(active))
	for gw := range active {
		sortedGateways = append(sortedGateways, gw)
	}
	sort.Slice(sortedGateways, func(i, j int) bool {
		if sortedGateways[i].Namespace != sortedGateways[j].Namespace {
			return sortedGateways[i].Namespace < sortedGateways[j].Namespace
		}
		return sortedGateways[i].Name < sortedGateways[j].Name
	})
	for _, gw := range sortedGateways {
		if !gatewayControlledByAgw(ctx, agw, gw) {
			continue
		}
		ps := inf.ParentStatus{
			ParentRef: inf.ParentReference{
				Kind:      inf.Kind(wellknown.GatewayKind),
				Namespace: inf.Namespace(gw.Namespace),
				Name:      inf.ObjectName(gw.Name),
			},
			ControllerName: inf.ControllerName(agw.ControllerName),
		}
		accepted := true
		if state, ok := acceptedByParent[gw]; ok {
			switch {
			case state.anyAccepted:
				accepted = true
			case state.anyRejected:
				accepted = false
			}
		}
		if accepted {
			meta.SetStatusCondition(&ps.Conditions, buildAcceptedCondition(pool.Generation, agw.ControllerName))
		} else {
			meta.SetStatusCondition(&ps.Conditions, buildNotAcceptedCondition(pool.Generation))
		}
		meta.SetStatusCondition(&ps.Conditions, buildResolvedRefsCondition(pool.Generation, errs))
		parents = append(parents, ps)
	}

	if len(errs) != 0 {
		ps := inf.ParentStatus{
			ParentRef: inf.ParentReference{
				Kind: inf.Kind(defaultInfPoolStatusKind),
				Name: inf.ObjectName(defaultInfPoolStatusName),
			},
		}
		meta.SetStatusCondition(&ps.Conditions, buildResolvedRefsCondition(pool.Generation, errs))
		parents = append(parents, ps)
	}

	return &inf.InferencePoolStatus{Parents: parents}
}

func gatewayControlledByAgw(ctx krt.HandlerContext, agw *AgwCollections, gw types.NamespacedName) bool {
	gwObj := ptr.Flatten(krt.FetchOne(ctx, agw.Gateways, krt.FilterObjectName(gw)))
	if gwObj == nil {
		return false
	}
	gcKey := types.NamespacedName{Name: string(gwObj.Spec.GatewayClassName)}
	gc := ptr.Flatten(krt.FetchOne(ctx, agw.GatewayClasses, krt.FilterObjectName(gcKey)))
	if gc == nil {
		return false
	}
	if gc.Spec.ControllerName != gwv1.GatewayController(agw.ControllerName) {
		return false
	}
	return true
}

func routeUsesPool(rt *gwv1.HTTPRoute, ns, name string) bool {
	for _, rule := range rt.Spec.Rules {
		for _, be := range rule.BackendRefs {
			group := inf.GroupVersion.Group
			if be.Group != nil {
				group = string(*be.Group)
			}
			kind := wellknown.InferencePoolKind
			if be.Kind != nil {
				kind = string(*be.Kind)
			}
			if be.Namespace != nil && string(*be.Namespace) != ns {
				continue
			}
			if group == inf.GroupVersion.Group &&
				kind == wellknown.InferencePoolKind &&
				be.Name == gwv1.ObjectName(name) {
				return true
			}
		}
	}
	return false
}

func validateInferencePool(ctx krt.HandlerContext, agw *AgwCollections, pool *inf.InferencePool) []error {
	var errs []error
	ext := pool.Spec.EndpointPickerRef

	if ext.Group != nil && *ext.Group != "" {
		errs = append(errs, fmt.Errorf(errInvalidGroupFormat, *ext.Group))
	}
	if ext.Kind != wellknown.ServiceKind {
		errs = append(errs, fmt.Errorf(errInvalidKindFormat, ext.Kind))
	}
	if len(pool.Spec.TargetPorts) != 1 {
		errs = append(errs, fmt.Errorf(errInvalidOneTargetPort))
	}

	if pool.Spec.EndpointPickerRef.Port == nil {
		errs = append(errs, fmt.Errorf(errPortRequired))
		return errs
	}

	svcNN := types.NamespacedName{Namespace: pool.Namespace, Name: string(ext.Name)}
	svc := ptr.Flatten(krt.FetchOne(ctx, agw.Services, krt.FilterKey(svcNN.String())))
	if svc == nil {
		errs = append(errs, fmt.Errorf(errServiceNotFoundFormat, pool.Namespace, ext.Name))
		return errs
	}

	if svc.Spec.Type == corev1.ServiceTypeExternalName {
		errs = append(errs, fmt.Errorf(errExternalNameNotAllowed))
	}

	found := false
	eppPort := int32(pool.Spec.EndpointPickerRef.Port.Number)
	for _, sp := range svc.Spec.Ports {
		proto := sp.Protocol
		if proto == "" {
			proto = corev1.ProtocolTCP
		}
		if sp.Port == eppPort && proto == corev1.ProtocolTCP {
			found = true
			break
		}
	}
	if !found {
		errs = append(errs, fmt.Errorf(errTCPPortNotFoundFormat, eppPort, pool.Namespace, ext.Name))
	}

	return errs
}

func buildAcceptedCondition(gen int64, controllerName string) metav1.Condition {
	return metav1.Condition{
		Type:               string(inf.InferencePoolConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(inf.InferencePoolReasonAccepted),
		Message:            fmt.Sprintf("InferencePool has been accepted by controller %s", controllerName),
		ObservedGeneration: gen,
		LastTransitionTime: metav1.Now(),
	}
}

func buildNotAcceptedCondition(gen int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(inf.InferencePoolConditionAccepted),
		Status:             metav1.ConditionFalse,
		Reason:             string(inf.InferencePoolReasonHTTPRouteNotAccepted),
		Message:            "InferencePool is referenced by a route that is not accepted",
		ObservedGeneration: gen,
		LastTransitionTime: metav1.Now(),
	}
}

func buildResolvedRefsCondition(gen int64, errs []error) metav1.Condition {
	cond := metav1.Condition{
		Type:               string(inf.InferencePoolConditionResolvedRefs),
		ObservedGeneration: gen,
		LastTransitionTime: metav1.Now(),
	}
	if len(errs) == 0 {
		cond.Status = metav1.ConditionTrue
		cond.Reason = string(inf.InferencePoolReasonResolvedRefs)
		cond.Message = "All InferencePool references have been resolved"
		return cond
	}
	var prefix string
	if len(errs) == 1 {
		prefix = "error:"
	} else {
		prefix = fmt.Sprintf("InferencePool has %d errors:", len(errs))
	}
	msgs := make([]string, 0, len(errs))
	for _, err := range errs {
		msgs = append(msgs, err.Error())
	}
	cond.Status = metav1.ConditionFalse
	cond.Reason = string(inf.InferencePoolReasonInvalidExtensionRef)
	cond.Message = fmt.Sprintf("%s %s", prefix, strings.Join(msgs, "; "))
	return cond
}
