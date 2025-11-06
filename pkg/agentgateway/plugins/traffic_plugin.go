package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agentgateway/agentgateway/go/api"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/google/cel-go/cel"
	"google.golang.org/protobuf/types/known/durationpb"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

const (
	extauthPolicySuffix         = ":extauth"
	extprocPolicySuffix         = ":extproc"
	rbacPolicySuffix            = ":rbac"
	localRateLimitPolicySuffix  = ":rl-local"
	globalRateLimitPolicySuffix = ":rl-global"
	transformationPolicySuffix  = ":transformation"
	csrfPolicySuffix            = ":csrf"
	corsPolicySuffix            = ":cors"
	headerModifierPolicySuffix  = ":header-modifier"
	hostnameRewritePolicySuffix = ":hostname-rewrite"
	retryPolicySuffix           = ":retry"
	timeoutPolicySuffix         = ":timeout"
)

var logger = logging.New("agentgateway/plugins")

// Shared CEL environment for expression validation
var celEnv *cel.Env

func init() {
	var err error
	celEnv, err = cel.NewEnv()
	if err != nil {
		logger.Error("failed to create CEL environment", "error", err)
		// Optionally, set celEnv to a default or nil value
		celEnv = nil // or some default configuration
	}
}

// convertStatusCollection converts the specific TrafficPolicy status collection
// to the generic controllers.Object status collection expected by the interface
func convertStatusCollection(col krt.Collection[krt.ObjectWithStatus[*v1alpha1.AgentgatewayPolicy, gwv1.PolicyStatus]]) krt.StatusCollection[controllers.Object, gwv1.PolicyStatus] {
	return krt.MapCollection(col, func(item krt.ObjectWithStatus[*v1alpha1.AgentgatewayPolicy, gwv1.PolicyStatus]) krt.ObjectWithStatus[controllers.Object, gwv1.PolicyStatus] {
		return krt.ObjectWithStatus[controllers.Object, gwv1.PolicyStatus]{
			Obj:    controllers.Object(item.Obj),
			Status: item.Status,
		}
	})
}

// NewAgentPlugin creates a new AgentgatewayPolicy plugin
func NewAgentPlugin(agw *AgwCollections) AgwPlugin {
	col := krt.WrapClient(kclient.NewFilteredDelayed[*v1alpha1.AgentgatewayPolicy](
		agw.Client,
		wellknown.AgentgatewayPolicyGVR,
		kclient.Filter{ObjectFilter: agw.Client.ObjectFilter()},
	), agw.KrtOpts.ToOptions("AgentgatewayPolicy")...)
	policyStatusCol, policyCol := krt.NewStatusManyCollection(col, func(krtctx krt.HandlerContext, policyCR *v1alpha1.AgentgatewayPolicy) (
		*gwv1.PolicyStatus,
		[]AgwPolicy,
	) {
		return TranslateAgentgatewayPolicy(krtctx, policyCR, agw)
	})

	return AgwPlugin{
		ContributesPolicies: map[schema.GroupKind]PolicyPlugin{
			wellknown.AgentgatewayPolicyGVK.GroupKind(): {
				Policies:       policyCol,
				PolicyStatuses: convertStatusCollection(policyStatusCol),
			},
		},
		ExtraHasSynced: func() bool {
			return policyCol.HasSynced() && policyStatusCol.HasSynced()
		},
	}
}

type PolicyCtx struct {
	Krt         krt.HandlerContext
	Collections *AgwCollections
}

type ResolvedTarget struct {
	AgentgatewayTarget *api.PolicyTarget
	GatewayTarget      gwv1.ParentReference
}

// TranslateAgentgatewayPolicy generates policies for a single traffic policy
func TranslateAgentgatewayPolicy(
	ctx krt.HandlerContext,
	policy *v1alpha1.AgentgatewayPolicy,
	agw *AgwCollections,
) (*gwv1.PolicyStatus, []AgwPolicy) {
	var agwPolicies []AgwPolicy

	pctx := PolicyCtx{Krt: ctx, Collections: agw}

	var policyTargets []ResolvedTarget
	// TODO: add selectors
	for _, target := range policy.Spec.TargetRefs {
		var policyTarget *api.PolicyTarget
		// Build a base ParentReference for status

		gk := schema.GroupKind{
			Group: string(target.Group),
			Kind:  string(target.Kind),
		}
		parentRef := gwv1.ParentReference{
			Name:      target.Name,
			Namespace: ptr.Of(gwv1.Namespace(policy.Namespace)),
			Group:     ptr.Of(gwv1.Group(gk.Group)),
			Kind:      ptr.Of(gwv1.Kind(gk.Kind)),
		}
		if target.SectionName != nil {
			parentRef.SectionName = target.SectionName
		}
		// TODO: add support for XListenerSet
		switch gk {
		case wellknown.GatewayGVK.GroupKind():
			policyTarget = &api.PolicyTarget{
				Kind: &api.PolicyTarget_Gateway{
					Gateway: utils.InternalGatewayName(policy.Namespace, string(target.Name), ""),
				},
			}
			if target.SectionName != nil {
				policyTarget = &api.PolicyTarget{
					Kind: &api.PolicyTarget_Listener{
						Listener: utils.InternalGatewayName(policy.Namespace, string(target.Name), string(*target.SectionName)),
					},
				}
			}

		case wellknown.HTTPRouteGVK.GroupKind():
			policyTarget = &api.PolicyTarget{
				Kind: &api.PolicyTarget_Route{
					Route: utils.InternalRouteRuleName(policy.Namespace, string(target.Name), ""),
				},
			}
			if target.SectionName != nil {
				policyTarget = &api.PolicyTarget{
					Kind: &api.PolicyTarget_RouteRule{
						RouteRule: utils.InternalRouteRuleName(policy.Namespace, string(target.Name), string(*target.SectionName)),
					},
				}
			}

		case wellknown.BackendGVK.GroupKind():
			policyTarget = &api.PolicyTarget{
				Kind: &api.PolicyTarget_Backend{
					Backend: utils.InternalBackendName(policy.Namespace, string(target.Name), ""),
				},
			}
			if target.SectionName != nil {
				policyTarget = &api.PolicyTarget{
					Kind: &api.PolicyTarget_SubBackend{
						SubBackend: utils.InternalBackendName(policy.Namespace, string(target.Name), string(*target.SectionName)),
					},
				}
			}

		case wellknown.ServiceGVK.GroupKind():
			hostname := kubeutils.GetServiceHostname(string(target.Name), policy.Namespace)
			policyTarget = &api.PolicyTarget{
				Kind: &api.PolicyTarget_Service{
					Service: policy.Namespace + "/" + hostname,
				},
			}
			if target.SectionName != nil {
				policyTarget = &api.PolicyTarget{
					Kind: &api.PolicyTarget_Backend{
						Backend: fmt.Sprintf("service/%s/%s:%s", policy.Namespace, hostname, *target.SectionName),
					},
				}
			}

			// TODO: inferencepool

		default:
			// TODO(npolshak): support attaching policies to k8s services, serviceentries, and other backends
			logger.Warn("unsupported target kind", "kind", target.Kind, "policy", policy.Name)
			continue
		}
		policyTargets = append(policyTargets, ResolvedTarget{
			AgentgatewayTarget: policyTarget,
			GatewayTarget:      parentRef,
		})
	}

	var ancestors []gwv1.PolicyAncestorStatus
	for _, policyTarget := range policyTargets {
		translatedPolicies, err := translatePolicyToAgw(pctx, policy, policyTarget.AgentgatewayTarget)
		agwPolicies = append(agwPolicies, translatedPolicies...)
		var conds []metav1.Condition
		if err != nil {
			// If we produced some policies alongside errors, treat as partial validity
			if len(translatedPolicies) > 0 {
				meta.SetStatusCondition(&conds, metav1.Condition{
					Type:    string(v1alpha1.PolicyConditionAccepted),
					Status:  metav1.ConditionTrue,
					Reason:  string(v1alpha1.PolicyReasonPartiallyValid),
					Message: err.Error(),
				})
			} else {
				// No policies produced and error present -> invalid
				meta.SetStatusCondition(&conds, metav1.Condition{
					Type:    string(v1alpha1.PolicyConditionAccepted),
					Status:  metav1.ConditionTrue,
					Reason:  string(v1alpha1.PolicyReasonInvalid),
					Message: err.Error(),
				})
				meta.SetStatusCondition(&conds, metav1.Condition{
					Type:    string(v1alpha1.PolicyConditionAttached),
					Status:  metav1.ConditionFalse,
					Reason:  string(v1alpha1.PolicyReasonPending),
					Message: "Policy is not attached due to invalid status",
				})
			}
		} else {
			// Check for partial validity
			// Build success conditions per ancestor
			meta.SetStatusCondition(&conds, metav1.Condition{
				Type:    string(v1alpha1.PolicyConditionAccepted),
				Status:  metav1.ConditionTrue,
				Reason:  string(v1alpha1.PolicyReasonValid),
				Message: reporter.PolicyAcceptedMsg,
			})
			meta.SetStatusCondition(&conds, metav1.Condition{
				Type:    string(v1alpha1.PolicyConditionAttached),
				Status:  metav1.ConditionTrue,
				Reason:  string(v1alpha1.PolicyReasonAttached),
				Message: reporter.PolicyAttachedMsg,
			})
		}
		// TODO: validate the target exists with dataplane https://github.com/kgateway-dev/kgateway/issues/12275
		// Ensure LastTransitionTime is set for all conditions
		for i := range conds {
			if conds[i].LastTransitionTime.IsZero() {
				conds[i].LastTransitionTime = metav1.Now()
			}
		}
		// Only append valid ancestors: require non-empty controllerName and parentRef name
		if agw.ControllerName != "" && string(policyTarget.GatewayTarget.Name) != "" {
			ancestors = append(ancestors, gwv1.PolicyAncestorStatus{
				AncestorRef:    policyTarget.GatewayTarget,
				ControllerName: v1alpha2.GatewayController(agw.ControllerName),
				Conditions:     conds,
			})
		}
	}

	// Build final status from accumulated ancestors
	status := gwv1.PolicyStatus{Ancestors: ancestors}

	if len(status.Ancestors) > 15 {
		ignored := status.Ancestors[15:]
		status.Ancestors = status.Ancestors[:15]
		status.Ancestors = append(status.Ancestors, gwv1.PolicyAncestorStatus{
			AncestorRef: gwv1.ParentReference{
				Group: ptr.Of(gwv1.Group("gateway.kgateway.dev")),
				Name:  "StatusSummary",
			},
			ControllerName: gwv1.GatewayController(agw.ControllerName),
			Conditions: []metav1.Condition{
				{
					Type:    "StatusSummarized",
					Status:  metav1.ConditionTrue,
					Reason:  "StatusSummary",
					Message: fmt.Sprintf("%d AncestorRefs ignored due to max status size", len(ignored)),
				},
			},
		})
	}

	// sort all parents for consistency with Equals and for Update
	// match sorting semantics of istio/istio, see:
	// https://github.com/istio/istio/blob/6dcaa0206bcaf20e3e3b4e45e9376f0f96365571/pilot/pkg/config/kube/gateway/conditions.go#L188-L193
	slices.SortStableFunc(status.Ancestors, func(a, b gwv1.PolicyAncestorStatus) int {
		return strings.Compare(reports.ParentString(a.AncestorRef), reports.ParentString(b.AncestorRef))
	})

	return &status, agwPolicies
}

// translateTrafficPolicyToAgw converts a TrafficPolicy to agentgateway Policy resources
func translatePolicyToAgw(
	ctx PolicyCtx,
	policy *v1alpha1.AgentgatewayPolicy,
	policyTarget *api.PolicyTarget,
) ([]AgwPolicy, error) {
	agwPolicies := make([]AgwPolicy, 0)
	var errs []error

	frontend, err := translateFrontendPolicyToAgw(policy, policyTarget)
	agwPolicies = append(agwPolicies, frontend...)
	if err != nil {
		errs = append(errs, err)
	}

	traffic, err := translateTrafficPolicyToAgw(ctx, policy, policyTarget)
	agwPolicies = append(agwPolicies, traffic...)
	if err != nil {
		errs = append(errs, err)
	}

	backend, err := translateBackendPolicyToAgw(ctx, policy, policyTarget)
	agwPolicies = append(agwPolicies, backend...)
	if err != nil {
		errs = append(errs, err)
	}

	return agwPolicies, errors.Join(errs...)
}

func translateTrafficPolicyToAgw(
	ctx PolicyCtx,
	policy *v1alpha1.AgentgatewayPolicy,
	policyTarget *api.PolicyTarget,
) ([]AgwPolicy, error) {
	traffic := policy.Spec.Traffic
	if traffic == nil {
		return nil, nil
	}

	agwPolicies := make([]AgwPolicy, 0)
	var errs []error

	// Generate a base policy name from the TrafficPolicy reference
	policyName := getTrafficPolicyName(policy.Namespace, policy.Name)

	// Convert ExtAuth policy if present
	if traffic.ExtAuth != nil {
		extAuthPolicies, err := processExtAuthPolicy(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing ExtAuth policy", "error", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, extAuthPolicies...)
	}

	// Convert ExtProc policy if present
	if traffic.ExtProc != nil {
		extProcPolicies, err := processExtProcPolicy(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing ExtProc policy", "error", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, extProcPolicies...)
	}

	// Convert Authorization policy if present
	if traffic.Authorization != nil {
		rbacPolicies := processAuthorizationPolicy(policy, policyName, policyTarget)
		agwPolicies = append(agwPolicies, rbacPolicies...)
	}

	// Process RateLimit policies if present
	if traffic.RateLimit != nil {
		rateLimitPolicies, err := processRateLimitPolicy(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing rate limit policy", "error", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, rateLimitPolicies...)
	}

	// Process transformation policies if present
	if traffic.Transformation != nil {
		transformationPolicies, err := processTransformationPolicy(policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing transformation policy", "error", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, transformationPolicies...)
	}

	// Process CSRF policies if present
	if traffic.Csrf != nil {
		csrfPolicies := processCSRFPolicy(policy, policyName, policyTarget)
		agwPolicies = append(agwPolicies, csrfPolicies...)
	}

	if traffic.Cors != nil {
		corsPolicies := processCorsPolicy(policy, policyName, policyTarget)
		agwPolicies = append(agwPolicies, corsPolicies...)
	}

	if traffic.HeaderModifiers != nil {
		headerModifiersPolicies := processHeaderModifierPolicy(policy, policyName, policyTarget)
		agwPolicies = append(agwPolicies, headerModifiersPolicies...)
	}

	if traffic.HostnameRewrite != nil {
		hostnameRewritePolicies, err := processHostnameRewritePolicy(policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing HostnameRewrite policy", "error", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, hostnameRewritePolicies...)
	}

	if traffic.Timeouts != nil {
		timeoutsPolicies := processTimeoutPolicy(policy, policyName, policyTarget)
		agwPolicies = append(agwPolicies, timeoutsPolicies...)
	}

	if traffic.Retry != nil {
		retriesPolicies := processRetriesPolicy(policy, policyName, policyTarget)
		agwPolicies = append(agwPolicies, retriesPolicies...)
	}

	// TODO:
	// TODO: phase

	return agwPolicies, errors.Join(errs...)
}

func processRetriesPolicy(policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) []AgwPolicy {
	retry := policy.Spec.Traffic.Retry
	translatedRetry := &api.Retry{}

	if retry.StatusCodes != nil {
		for _, c := range retry.StatusCodes {
			translatedRetry.RetryStatusCodes = append(translatedRetry.RetryStatusCodes, int32(c)) //nolint:gosec // G115: HTTP status codes are always positive integers (100-599)
		}
	}

	if retry.BackoffBaseInterval != nil {
		translatedRetry.Backoff = durationpb.New(retry.BackoffBaseInterval.Duration)
	}

	translatedRetry.Attempts = retry.Attempts

	retryPolicy := &api.Policy{
		Name:   name + retryPolicySuffix + attachmentName(target),
		Target: target,
		Kind: &api.Policy_Traffic{
			Traffic: &api.TrafficPolicySpec{
				Kind: &api.TrafficPolicySpec_Retry{Retry: translatedRetry},
			},
		},
	}

	logger.Debug("generated Timeout policy",
		"policy", policy.Name,
		"agentgateway_policy", retryPolicy.Name,
		"target", target)

	return []AgwPolicy{{Policy: retryPolicy}}
}

func processTimeoutPolicy(policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) []AgwPolicy {
	timeout := policy.Spec.Traffic.Timeouts
	timeoutPolicy := &api.Policy{
		Name:   name + timeoutPolicySuffix + attachmentName(target),
		Target: target,
		Kind: &api.Policy_Traffic{
			Traffic: &api.TrafficPolicySpec{
				Kind: &api.TrafficPolicySpec_Timeout{Timeout: &api.Timeout{
					Request: durationpb.New(timeout.Request.Duration),
				}},
			},
		},
	}

	logger.Debug("generated Timeout policy",
		"policy", policy.Name,
		"agentgateway_policy", timeoutPolicy.Name,
		"target", target)

	return []AgwPolicy{{Policy: timeoutPolicy}}
}

func processHostnameRewritePolicy(policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	// TODO
	return nil, nil
}

func processHeaderModifierPolicy(policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) []AgwPolicy {
	var policies []AgwPolicy
	headerModifier := policy.Spec.Traffic.HeaderModifiers

	var headerModifierPolicyRequest, headerModifierPolicyResponse *api.Policy
	if headerModifier.Request != nil {
		headerModifierPolicyRequest = &api.Policy{
			Name:   name + headerModifierPolicySuffix + attachmentName(target),
			Target: target,
			Kind: &api.Policy_Traffic{
				Traffic: &api.TrafficPolicySpec{
					Kind: &api.TrafficPolicySpec_RequestHeaderModifier{RequestHeaderModifier: &api.HeaderModifier{
						Add:    headerListToAgw(headerModifier.Request.Add),
						Set:    headerListToAgw(headerModifier.Request.Set),
						Remove: headerModifier.Request.Remove,
					}},
				},
			},
		}
		logger.Debug("generated HeaderModifier policy",
			"policy", policy.Name,
			"agentgateway_policy", headerModifierPolicyRequest.Name,
			"target", target)
		policies = append(policies, AgwPolicy{Policy: headerModifierPolicyRequest})
	}

	if headerModifier.Response != nil {
		headerModifierPolicyResponse = &api.Policy{
			Name:   name + headerModifierPolicySuffix + attachmentName(target),
			Target: target,
			Kind: &api.Policy_Traffic{
				Traffic: &api.TrafficPolicySpec{
					Kind: &api.TrafficPolicySpec_ResponseHeaderModifier{ResponseHeaderModifier: &api.HeaderModifier{
						Add:    headerListToAgw(headerModifier.Response.Add),
						Set:    headerListToAgw(headerModifier.Response.Set),
						Remove: headerModifier.Response.Remove,
					}},
				},
			},
		}
		logger.Debug("generated HeaderModifier policy",
			"policy", policy.Name,
			"agentgateway_policy", headerModifierPolicyResponse.Name,
			"target", target)
		policies = append(policies, AgwPolicy{Policy: headerModifierPolicyResponse})
	}

	return policies
}

func processCorsPolicy(policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) []AgwPolicy {
	cors := policy.Spec.Traffic.Cors
	corsPolicy := &api.Policy{
		Name:   name + corsPolicySuffix + attachmentName(target),
		Target: target,
		Kind: &api.Policy_Traffic{
			Traffic: &api.TrafficPolicySpec{
				Kind: &api.TrafficPolicySpec_Cors{Cors: &api.CORS{
					AllowCredentials: ptr.OrEmpty(cors.AllowCredentials),
					AllowHeaders:     slices.Map(cors.AllowHeaders, func(h gwv1.HTTPHeaderName) string { return string(h) }),
					AllowMethods:     slices.Map(cors.AllowMethods, func(m gwv1.HTTPMethodWithWildcard) string { return string(m) }),
					AllowOrigins:     slices.Map(cors.AllowOrigins, func(o gwv1.CORSOrigin) string { return string(o) }),
					ExposeHeaders:    slices.Map(cors.ExposeHeaders, func(h gwv1.HTTPHeaderName) string { return string(h) }),
					MaxAge: &duration.Duration{
						Seconds: int64(cors.MaxAge),
					},
				}},
			},
		},
	}

	logger.Debug("generated Cors policy",
		"policy", policy.Name,
		"agentgateway_policy", corsPolicy.Name,
		"target", target)

	return []AgwPolicy{{Policy: corsPolicy}}
}

// processExtAuthPolicy processes ExtAuth configuration and creates corresponding agentgateway policies
func processExtAuthPolicy(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, policyName string, policyTarget *api.PolicyTarget) ([]AgwPolicy, error) {
	extAuth := policy.Spec.Traffic.ExtAuth

	be, err := buildBackendRef(ctx, extAuth.BackendRef, policy.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to build extAuth: %v", err)
	}
	spec := &api.TrafficPolicySpec_ExternalAuth{
		Target:  be,
		Context: extAuth.ContextExtensions,
	}
	if b := extAuth.ForwardBody; b != nil {
		spec.IncludeRequestBody = &api.TrafficPolicySpec_ExternalAuth_BodyOptions{
			// nolint:gosec // G115: kubebuilder validation ensures safe for uint32
			MaxRequestBytes: uint32(b.MaxSize),
			// Currently the default, see https://github.com/kubernetes-sigs/gateway-api/issues/4198
			AllowPartialMessage: true,
			// TODO: should we allow config?
			PackAsBytes: false,
		}
	}

	extauthPolicy := &api.Policy{
		Name:   policyName + extauthPolicySuffix + attachmentName(policyTarget),
		Target: policyTarget,
		Kind: &api.Policy_Traffic{
			Traffic: &api.TrafficPolicySpec{
				Phase: phase(policy),
				Kind: &api.TrafficPolicySpec_ExtAuthz{
					ExtAuthz: spec,
				},
			},
		},
	}

	logger.Debug("generated ExtAuth policy",
		"policy", policy.Name,
		"agentgateway_policy", extauthPolicy.Name,
		"target", policyTarget)

	return []AgwPolicy{{Policy: extauthPolicy}}, nil
}

// processExtProcPolicy processes ExtProc configuration and creates corresponding agentgateway policies
func processExtProcPolicy(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, policyName string, policyTarget *api.PolicyTarget) ([]AgwPolicy, error) {
	extProc := policy.Spec.Traffic.ExtAuth

	be, err := buildBackendRef(ctx, extProc.BackendRef, policy.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to build extProc: %v", err)
	}
	spec := &api.TrafficPolicySpec_ExtProc{
		Target: be,
	}

	extprocPolicy := &api.Policy{
		Name:   policyName + extprocPolicySuffix + attachmentName(policyTarget),
		Target: policyTarget,
		Kind: &api.Policy_Traffic{
			Traffic: &api.TrafficPolicySpec{
				Phase: phase(policy),
				Kind: &api.TrafficPolicySpec_ExtProc_{
					ExtProc: spec,
				},
			},
		},
	}

	logger.Debug("generated ExtProc policy",
		"policy", policy.Name,
		"agentgateway_policy", extprocPolicy.Name,
		"target", policyTarget)

	return []AgwPolicy{{Policy: extprocPolicy}}, nil
}

func phase(policy *v1alpha1.AgentgatewayPolicy) api.TrafficPolicySpec_PolicyPhase {
	var phase api.TrafficPolicySpec_PolicyPhase
	if policy.Spec.Traffic.Phase != nil {
		switch *policy.Spec.Traffic.Phase {
		case v1alpha1.PolicyPhasePreRouting:
			phase = api.TrafficPolicySpec_ROUTE
		case v1alpha1.PolicyPhasePostRouting:
			phase = api.TrafficPolicySpec_GATEWAY
		}
	}
	return phase
}

func cast[T ~string](items []T) []string {
	return slices.Map(items, func(item T) string {
		return string(item)
	})
}

// processAuthorizationPolicy processes Authorization configuration and creates corresponding Agw policies
func processAuthorizationPolicy(
	policy *v1alpha1.AgentgatewayPolicy,
	policyName string,
	policyTarget *api.PolicyTarget,
) []AgwPolicy {
	auth := policy.Spec.Traffic.Authorization
	var allowPolicies, denyPolicies []string
	if auth.Action == v1alpha1.AuthorizationPolicyActionDeny {
		denyPolicies = append(denyPolicies, cast(auth.Policy.MatchExpressions)...)
	} else {
		allowPolicies = append(allowPolicies, cast(auth.Policy.MatchExpressions)...)
	}

	pol := &api.Policy{
		Name:   policyName + rbacPolicySuffix + attachmentName(policyTarget),
		Target: policyTarget,
		Kind: &api.Policy_Traffic{
			Traffic: &api.TrafficPolicySpec{
				Kind: &api.TrafficPolicySpec_Authorization{
					Authorization: &api.TrafficPolicySpec_RBAC{
						Allow: allowPolicies,
						Deny:  denyPolicies,
					},
				},
			},
		},
	}

	logger.Debug("generated Authorization policy",
		"policy", policy.Name,
		"agentgateway_policy", pol.Name,
		"target", policyTarget)

	return []AgwPolicy{{Policy: pol}}
}

func getFrontendPolicyName(trafficPolicyNs, trafficPolicyName string) string {
	return fmt.Sprintf("frontend/%s/%s", trafficPolicyNs, trafficPolicyName)
}

func getBackendPolicyName(trafficPolicyNs, trafficPolicyName string) string {
	return fmt.Sprintf("backend/%s/%s", trafficPolicyNs, trafficPolicyName)
}

func getTrafficPolicyName(trafficPolicyNs, trafficPolicyName string) string {
	return fmt.Sprintf("traffic/%s/%s", trafficPolicyNs, trafficPolicyName)
}

// processRateLimitPolicy processes RateLimit configuration and creates corresponding agentgateway policies
func processRateLimitPolicy(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, policyName string, policyTarget *api.PolicyTarget) ([]AgwPolicy, error) {
	var agwPolicies []AgwPolicy
	var errs []error
	rl := policy.Spec.Traffic.RateLimit

	// Process local rate limiting if present
	if rl.Local != nil {
		localPolicy := processLocalRateLimitPolicy(rl.Local, policyName, policyTarget)
		if localPolicy != nil {
			agwPolicies = append(agwPolicies, *localPolicy)
		}
	}

	// Process global rate limiting if present
	if rl.Global != nil {
		globalPolicy, err := processGlobalRateLimitPolicy(ctx, *rl.Global, policy, policyName, policyTarget)
		if globalPolicy != nil && err == nil {
			agwPolicies = append(agwPolicies, *globalPolicy)
		} else {
			errs = append(errs, err)
		}
	}

	return agwPolicies, errors.Join(errs...)
}

// processLocalRateLimitPolicy processes local rate limiting configuration
func processLocalRateLimitPolicy(limits []v1alpha1.AgentLocalRateLimitPolicy, policyName string, policyTarget *api.PolicyTarget) *AgwPolicy {
	// TODO: support multiple
	limit := limits[0]

	rule := &api.TrafficPolicySpec_LocalRateLimit{
		Type: api.TrafficPolicySpec_LocalRateLimit_REQUEST,
	}
	var capacity uint64
	if limit.Requests != nil {
		capacity = uint64(*limit.Requests) //nolint:gosec // G115: kubebuilder validation ensures non-negative, safe for uint64
		rule.Type = api.TrafficPolicySpec_LocalRateLimit_REQUEST
	} else {
		capacity = uint64(*limit.Tokens) //nolint:gosec // G115: kubebuilder validation ensures non-negative, safe for uint64
		rule.Type = api.TrafficPolicySpec_LocalRateLimit_TOKEN
	}
	rule.MaxTokens = capacity + uint64(ptr.OrEmpty(limit.Burst)) //nolint:gosec // G115: Burst is non-negative, safe for uint64
	rule.TokensPerFill = capacity
	switch limit.Unit {
	case v1alpha1.LocalRateLimitUnitSeconds:
		rule.FillInterval = durationpb.New(time.Second)
	case v1alpha1.LocalRateLimitUnitMinutes:
		rule.FillInterval = durationpb.New(time.Minute)
	case v1alpha1.LocalRateLimitUnitHours:
		rule.FillInterval = durationpb.New(time.Hour)
	}

	localRateLimitPolicy := &api.Policy{
		Name:   policyName + localRateLimitPolicySuffix + attachmentName(policyTarget),
		Target: policyTarget,
		Kind: &api.Policy_Traffic{
			Traffic: &api.TrafficPolicySpec{
				Kind: &api.TrafficPolicySpec_LocalRateLimit_{
					LocalRateLimit: rule,
				},
			},
		},
	}

	return &AgwPolicy{Policy: localRateLimitPolicy}
}

func processGlobalRateLimitPolicy(
	ctx PolicyCtx,
	grl v1alpha1.AgentRateLimitPolicy,
	trafficPolicy *v1alpha1.AgentgatewayPolicy,
	policyName string,
	policyTarget *api.PolicyTarget,
) (*AgwPolicy, error) {
	be, err := buildBackendRef(ctx, grl.BackendRef, trafficPolicy.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to build global rate limit: %v", err)
	}
	// Translate descriptors
	descriptors := make([]*api.TrafficPolicySpec_RemoteRateLimit_Descriptor, 0, len(grl.Descriptors))
	for _, d := range grl.Descriptors {
		if agw := processRateLimitDescriptor(d); agw != nil {
			descriptors = append(descriptors, agw)
		}
	}

	// Build the RemoteRateLimit policy that agentgateway expects
	p := &api.Policy{
		Name:   policyName + globalRateLimitPolicySuffix + attachmentName(policyTarget),
		Target: policyTarget,
		Kind: &api.Policy_Traffic{
			Traffic: &api.TrafficPolicySpec{
				Kind: &api.TrafficPolicySpec_RemoteRateLimit_{
					RemoteRateLimit: &api.TrafficPolicySpec_RemoteRateLimit{
						Domain:      grl.Domain,
						Target:      be,
						Descriptors: descriptors,
					},
				},
			},
		},
	}

	return &AgwPolicy{Policy: p}, nil
}

func processRateLimitDescriptor(descriptor v1alpha1.AgentRateLimitDescriptor) *api.TrafficPolicySpec_RemoteRateLimit_Descriptor {
	entries := make([]*api.TrafficPolicySpec_RemoteRateLimit_Entry, 0, len(descriptor.Entries))

	for _, entry := range descriptor.Entries {
		entries = append(entries, &api.TrafficPolicySpec_RemoteRateLimit_Entry{
			Key:   entry.Name,
			Value: string(entry.Expression),
		})
	}

	return &api.TrafficPolicySpec_RemoteRateLimit_Descriptor{
		Entries: entries,
		Type:    api.TrafficPolicySpec_RemoteRateLimit_REQUESTS,
	}
}

func buildBackendRef(ctx PolicyCtx, ref gwv1.BackendObjectReference, defaultNS string) (*api.BackendReference, error) {
	kind := ptr.OrDefault(ref.Kind, wellknown.ServiceKind)
	group := ptr.OrDefault(ref.Group, "")
	gk := schema.GroupKind{
		Group: string(group),
		Kind:  string(kind),
	}
	namespace := string(ptr.OrDefault(ref.Namespace, gwv1.Namespace(defaultNS)))
	switch gk {
	case wellknown.InferencePoolGVK.GroupKind():
		if strings.Contains(string(ref.Name), ".") {
			return nil, errors.New("service name invalid; the name of the InferencePool, not the hostname")
		}
		hostname := kubeutils.GetInferenceServiceHostname(string(ref.Name), namespace)
		key := namespace + "/" + string(ref.Name)
		svc := ptr.Flatten(krt.FetchOne(ctx.Krt, ctx.Collections.InferencePools, krt.FilterKey(key)))
		logger.Debug("found pull pool for service", "svc", svc, "key", key)
		if svc == nil {
			return nil, fmt.Errorf("unable to find the InferencePool %v", key)
		} else {
			return &api.BackendReference{
				Kind: &api.BackendReference_Service{
					Service: namespace + "/" + hostname,
				},
				// InferencePool only supports single port
				Port: uint32(svc.Spec.TargetPorts[0].Number), //nolint:gosec // G115: InferencePool TargetPort is int32 with validation 1-65535, always safe
			}, nil
		}
	case wellknown.ServiceGVK.GroupKind():
		port := ref.Port
		if strings.Contains(string(ref.Name), ".") {
			return nil, errors.New("service name invalid; the name of the Service, not the hostname")
		}
		hostname := kubeutils.GetServiceHostname(string(ref.Name), namespace)
		key := namespace + "/" + string(ref.Name)
		svc := ptr.Flatten(krt.FetchOne(ctx.Krt, ctx.Collections.Services, krt.FilterKey(key)))
		if svc == nil {
			return nil, fmt.Errorf("unable to find the Service %v", key)
		}
		// TODO: All kubernetes service types currently require a Port, so we do this for everything; consider making this per-type if we have future types
		// that do not require port.
		if port == nil {
			// "Port is required when the referent is a Kubernetes Service."
			return nil, errors.New("port is required for Service targets")
		}
		return &api.BackendReference{
			Kind: &api.BackendReference_Service{
				Service: namespace + "/" + hostname,
			},
			Port: uint32(*port), //nolint:gosec // G115: Gateway API PortNumber is int32 with validation 1-65535, always safe
		}, nil
	case wellknown.BackendGVK.GroupKind():
		key := namespace + "/" + string(ref.Name)
		be := ptr.Flatten(krt.FetchOne(ctx.Krt, ctx.Collections.Backends, krt.FilterKey(key)))
		if be == nil {
			return nil, fmt.Errorf("unable to find the Backend %v", key)
		}
		return &api.BackendReference{
			Kind: &api.BackendReference_Backend{
				Backend: key,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported backend %v", gk)
	}
}

func toJSONValue(value string) (string, error) {
	if json.Valid([]byte(value)) {
		return value, nil
	}

	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
		return "", fmt.Errorf("invalid JSON value: %s", value)
	}

	// Treat this as an unquoted string and marshal it to JSON
	marshaled, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(marshaled), nil
}

func processCSRFPolicy(policy *v1alpha1.AgentgatewayPolicy, policyName string, policyTarget *api.PolicyTarget) []AgwPolicy {
	csrf := policy.Spec.Traffic.Csrf
	csrfPolicy := &api.Policy{
		Name:   policyName + csrfPolicySuffix + attachmentName(policyTarget),
		Target: policyTarget,
		Kind: &api.Policy_Traffic{
			Traffic: &api.TrafficPolicySpec{
				Kind: &api.TrafficPolicySpec_Csrf{
					Csrf: &api.TrafficPolicySpec_CSRF{
						AdditionalOrigins: csrf.AdditionalOrigins,
					},
				},
			},
		},
	}

	return []AgwPolicy{{Policy: csrfPolicy}}
}

// processTransformationPolicy processes transformation configuration and creates corresponding Agw policies
func processTransformationPolicy(
	policy *v1alpha1.AgentgatewayPolicy,
	policyName string,
	policyTarget *api.PolicyTarget,
) ([]AgwPolicy, error) {
	var errs []error
	transformation := policy.Spec.Traffic.Transformation

	convertedReq, err := convertTransformSpec(transformation.Request)
	if err != nil {
		errs = append(errs, err)
	}
	convertedResp, err := convertTransformSpec(transformation.Response)
	if err != nil {
		errs = append(errs, err)
	}

	if convertedResp != nil || convertedReq != nil {
		transformationPolicy := &api.Policy{
			Name:   policyName + transformationPolicySuffix + attachmentName(policyTarget),
			Target: policyTarget,
			Kind: &api.Policy_Traffic{
				Traffic: &api.TrafficPolicySpec{
					Phase: phase(policy),
					Kind: &api.TrafficPolicySpec_Transformation{
						Transformation: &api.TrafficPolicySpec_TransformationPolicy{
							Request:  convertedReq,
							Response: convertedResp,
						},
					},
				},
			},
		}

		logger.Debug("generated transformation policy",
			"policy", policy.Name,
			"agentgateway_policy", transformationPolicy.Name,
			"target", policyTarget)
		return []AgwPolicy{{Policy: transformationPolicy}}, errors.Join(errs...)
	}
	return nil, errors.Join(errs...)
}

// convertTransformSpec converts transformation specs to agentgateway format
func convertTransformSpec(spec *v1alpha1.AgentTransform) (*api.TrafficPolicySpec_TransformationPolicy_Transform, error) {
	if spec == nil {
		return nil, nil
	}
	var errs []error
	var transform *api.TrafficPolicySpec_TransformationPolicy_Transform

	for _, header := range spec.Set {
		headerValue := header.Value
		if isCEL(headerValue) {
			if transform == nil {
				transform = &api.TrafficPolicySpec_TransformationPolicy_Transform{}
			}
			transform.Set = append(transform.Set, &api.TrafficPolicySpec_HeaderTransformation{
				Name:       string(header.Name),
				Expression: string(header.Value),
			})
		} else {
			errs = append(errs, fmt.Errorf("header value is not a valid CEL expression: %s", headerValue))
		}
	}

	for _, header := range spec.Add {
		headerValue := header.Value
		if isCEL(headerValue) {
			if transform == nil {
				transform = &api.TrafficPolicySpec_TransformationPolicy_Transform{}
			}
			transform.Add = append(transform.Add, &api.TrafficPolicySpec_HeaderTransformation{
				Name:       string(header.Name),
				Expression: string(header.Value),
			})
		} else {
			errs = append(errs, fmt.Errorf("invalid header value: %s", headerValue))
		}
	}

	if spec.Remove != nil {
		if transform == nil {
			transform = &api.TrafficPolicySpec_TransformationPolicy_Transform{}
		}
		transform.Remove = cast(spec.Remove)
	}

	if spec.Body != nil {
		// Handle body transformation if present
		bodyValue := *spec.Body
		if isCEL(bodyValue) {
			if transform == nil {
				transform = &api.TrafficPolicySpec_TransformationPolicy_Transform{}
			}
			transform.Body = &api.TrafficPolicySpec_BodyTransformation{
				Expression: string(bodyValue),
			}
		} else {
			errs = append(errs, fmt.Errorf("body value is not a valid CEL expression: %s", bodyValue))
		}
	}

	return transform, errors.Join(errs...)
}

// Checks if the expression is a valid CEL expression
func isCEL(expr v1alpha1.CELExpression) bool {
	_, iss := celEnv.Parse(string(expr))
	return iss.Err() == nil
}

func attachmentName(target *api.PolicyTarget) string {
	if target == nil {
		return ""
	}
	switch v := target.Kind.(type) {
	case *api.PolicyTarget_Gateway:
		return ":" + v.Gateway
	case *api.PolicyTarget_Listener:
		return ":" + v.Listener
	case *api.PolicyTarget_Route:
		return ":" + v.Route
	case *api.PolicyTarget_RouteRule:
		return ":" + v.RouteRule
	case *api.PolicyTarget_Backend:
		return ":" + v.Backend
	case *api.PolicyTarget_Service:
		return ":" + v.Service
	case *api.PolicyTarget_SubBackend:
		return ":" + v.SubBackend
	default:
		panic(fmt.Sprintf("unknown target kind %T", target))
	}
}

func headerListToAgw(hl []gwv1.HTTPHeader) []*api.Header {
	return slices.Map(hl, func(hl gwv1.HTTPHeader) *api.Header {
		return &api.Header{
			Name:  string(hl.Name),
			Value: hl.Value,
		}
	})
}
