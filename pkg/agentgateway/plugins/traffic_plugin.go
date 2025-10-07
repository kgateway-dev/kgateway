package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/agentgateway/agentgateway/go/api"
	"github.com/google/cel-go/cel"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	aiPolicySuffix              = ":ai"
	rbacPolicySuffix            = ":rbac"
	localRateLimitPolicySuffix  = ":rl-local"
	globalRateLimitPolicySuffix = ":rl-global"
	transformationPolicySuffix  = ":transformation"
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
func convertStatusCollection(col krt.Collection[krt.ObjectWithStatus[*v1alpha1.TrafficPolicy, v1alpha2.PolicyStatus]]) krt.StatusCollection[controllers.Object, v1alpha2.PolicyStatus] {
	// Use krt.NewCollection to transform the collection
	return krt.NewCollection(col, func(ctx krt.HandlerContext, item krt.ObjectWithStatus[*v1alpha1.TrafficPolicy, v1alpha2.PolicyStatus]) *krt.ObjectWithStatus[controllers.Object, v1alpha2.PolicyStatus] {
		return &krt.ObjectWithStatus[controllers.Object, v1alpha2.PolicyStatus]{
			Obj:    controllers.Object(item.Obj),
			Status: item.Status,
		}
	})
}

// NewTrafficPlugin creates a new TrafficPolicy plugin
func NewTrafficPlugin(agw *AgwCollections) AgwPlugin {
	col := krt.WrapClient(kclient.NewFiltered[*v1alpha1.TrafficPolicy](
		agw.Client,
		kclient.Filter{ObjectFilter: agw.Client.ObjectFilter()},
	), agw.KrtOpts.ToOptions("TrafficPolicy")...)
	policyStatusCol, policyCol := krt.NewStatusManyCollection(col, func(krtctx krt.HandlerContext, policyCR *v1alpha1.TrafficPolicy) (
		*v1alpha2.PolicyStatus,
		[]AgwPolicy,
	) {
		return TranslateTrafficPolicy(krtctx, agw.GatewayExtensions, agw.Backends, agw.Secrets, policyCR, agw.ControllerName)
	})

	return AgwPlugin{
		ContributesPolicies: map[schema.GroupKind]PolicyPlugin{
			wellknown.TrafficPolicyGVK.GroupKind(): {
				Policies:       policyCol,
				PolicyStatuses: convertStatusCollection(policyStatusCol),
			},
		},
		ExtraHasSynced: func() bool {
			return policyCol.HasSynced() && policyStatusCol.HasSynced()
		},
	}
}

// TranslateTrafficPolicy generates policies for a single traffic policy
func TranslateTrafficPolicy(
	ctx krt.HandlerContext,
	gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension],
	backends krt.Collection[*v1alpha1.Backend],
	secrets krt.Collection[*corev1.Secret],
	trafficPolicy *v1alpha1.TrafficPolicy,
	controllerName string,
) (*v1alpha2.PolicyStatus, []AgwPolicy) {
	var agwPolicies []AgwPolicy

	isMcpTarget := false
	var ancestors []v1alpha2.PolicyAncestorStatus
	for _, target := range trafficPolicy.Spec.TargetRefs {
		var policyTarget *api.PolicyTarget
		// Build a base ParentReference for status
		parentRef := gwv1.ParentReference{
			Name:      gwv1.ObjectName(target.Name),
			Namespace: ptr.To(gwv1.Namespace(trafficPolicy.Namespace)),
		}
		if target.SectionName != nil {
			parentRef.SectionName = (*gwv1.SectionName)(target.SectionName)
		}

		switch string(target.Kind) {
		case wellknown.GatewayKind:
			group := gwv1.Group(wellknown.GatewayGVK.Group)
			kind := gwv1.Kind(wellknown.GatewayGVK.Kind)
			parentRef.Group = &group
			parentRef.Kind = &kind
			policyTarget = &api.PolicyTarget{
				Kind: &api.PolicyTarget_Gateway{
					Gateway: utils.InternalGatewayName(trafficPolicy.Namespace, string(target.Name), ""),
				},
			}
			if target.SectionName != nil {
				policyTarget = &api.PolicyTarget{
					Kind: &api.PolicyTarget_Listener{
						Listener: utils.InternalGatewayName(trafficPolicy.Namespace, string(target.Name), string(*target.SectionName)),
					},
				}
			}

		case wellknown.HTTPRouteKind:
			group := gwv1.Group(wellknown.HTTPRouteGVK.Group)
			kind := gwv1.Kind(wellknown.HTTPRouteGVK.Kind)
			parentRef.Group = &group
			parentRef.Kind = &kind
			policyTarget = &api.PolicyTarget{
				Kind: &api.PolicyTarget_Route{
					Route: utils.InternalRouteRuleName(trafficPolicy.Namespace, string(target.Name), ""),
				},
			}
			if target.SectionName != nil {
				policyTarget = &api.PolicyTarget{
					Kind: &api.PolicyTarget_RouteRule{
						RouteRule: utils.InternalRouteRuleName(trafficPolicy.Namespace, string(target.Name), string(*target.SectionName)),
					},
				}
			}

		case wellknown.BackendGVK.Kind:
			// kgateway backend kind (MCP, AI, etc.)
			group := gwv1.Group(wellknown.BackendGVK.Group)
			kind := gwv1.Kind(wellknown.BackendGVK.Kind)
			parentRef.Group = &group
			parentRef.Kind = &kind

			// Look up the Backend referenced by the policy
			backendKey := getBackendKey(trafficPolicy.Namespace, string(target.Name))
			backend := krt.FetchOne(ctx, backends, krt.FilterKey(backendKey))
			if backend == nil {
				logger.Error("backend not found",
					"target", target.Name,
					"policy", client.ObjectKeyFromObject(trafficPolicy))
				// Return an error status when backend is not found
				conds := []metav1.Condition{}
				meta.SetStatusCondition(&conds, metav1.Condition{
					Type:    string(v1alpha1.PolicyConditionAccepted),
					Status:  metav1.ConditionFalse,
					Reason:  string(v1alpha1.PolicyReasonInvalid),
					Message: fmt.Sprintf("Backend %s not found", target.Name),
				})
				status := v1alpha2.PolicyStatus{
					Ancestors: []v1alpha2.PolicyAncestorStatus{
						{
							AncestorRef:    parentRef,
							ControllerName: v1alpha2.GatewayController(controllerName),
							Conditions:     conds,
						},
					},
				}
				return &status, nil
			}
			backendSpec := (*backend).Spec
			if backendSpec.Type == v1alpha1.BackendTypeMCP {
				isMcpTarget = true
				policyTarget = &api.PolicyTarget{
					Kind: &api.PolicyTarget_Backend{
						Backend: trafficPolicy.Namespace + "/" + string(target.Name),
					},
				}
			} else {
				logger.Warn("unsupported target kind. only MCP backends are supported",
					"kind", target.Kind,
					"policy", client.ObjectKeyFromObject(trafficPolicy))
				continue
			}
		default:
			// TODO(npolshak): support attaching policies to k8s services, serviceentries, and other backends
			logger.Warn("unsupported target kind", "kind", target.Kind, "policy", trafficPolicy.Name)
			continue
		}

		if policyTarget != nil {
			translatedPolicies, err := translateTrafficPolicyToAgw(ctx, gatewayExtensions, secrets, trafficPolicy, string(target.Name), policyTarget, isMcpTarget)
			agwPolicies = append(agwPolicies, translatedPolicies...)
			var conds []metav1.Condition
			// TODO: support partial translation statuses https://github.com/kgateway-dev/kgateway/issues/12413
			if err != nil {
				// Build success conditions per ancestor
				meta.SetStatusCondition(&conds, metav1.Condition{
					Type:    string(v1alpha1.PolicyConditionAccepted),
					Status:  metav1.ConditionFalse,
					Reason:  string(v1alpha1.PolicyReasonInvalid),
					Message: err.Error(),
				})
			} else {
				// Build success conditions per ancestor
				meta.SetStatusCondition(&conds, metav1.Condition{
					Type:    string(v1alpha1.PolicyConditionAccepted),
					Status:  metav1.ConditionTrue,
					Reason:  string(v1alpha1.PolicyReasonValid),
					Message: reporter.PolicyAcceptedMsg,
				})
			}
			// TODO: validate the target exists with dataplane https://github.com/kgateway-dev/kgateway/issues/12275
			meta.SetStatusCondition(&conds, metav1.Condition{
				Type:    string(v1alpha1.PolicyConditionAttached),
				Status:  metav1.ConditionTrue,
				Reason:  string(v1alpha1.PolicyReasonAttached),
				Message: reporter.PolicyAttachedMsg,
			})
			// Ensure LastTransitionTime is set for all conditions
			for i := range conds {
				if conds[i].LastTransitionTime.IsZero() {
					conds[i].LastTransitionTime = metav1.Now()
				}
			}
			// Only append valid ancestors: require non-empty controllerName and parentRef name
			if controllerName != "" && string(parentRef.Name) != "" {
				ancestors = append(ancestors, v1alpha2.PolicyAncestorStatus{
					AncestorRef:    parentRef,
					ControllerName: v1alpha2.GatewayController(controllerName),
					Conditions:     conds,
				})
			}
		}
	}

	// Build final status from accumulated ancestors
	status := v1alpha2.PolicyStatus{Ancestors: ancestors}

	if len(status.Ancestors) > 15 {
		ignored := status.Ancestors[15:]
		status.Ancestors = status.Ancestors[:15]
		status.Ancestors = append(status.Ancestors, v1alpha2.PolicyAncestorStatus{
			AncestorRef: gwv1.ParentReference{
				Group: ptr.To(gwv1.Group("gateway.kgateway.dev")),
				Name:  "StatusSummary",
			},
			ControllerName: gwv1.GatewayController(controllerName),
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
	slices.SortStableFunc(status.Ancestors, func(a, b v1alpha2.PolicyAncestorStatus) int {
		return strings.Compare(reports.ParentString(a.AncestorRef), reports.ParentString(b.AncestorRef))
	})

	return &status, agwPolicies
}

// translateTrafficPolicyToAgw converts a TrafficPolicy to agentgateway Policy resources
func translateTrafficPolicyToAgw(
	ctx krt.HandlerContext,
	gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension],
	secrets krt.Collection[*corev1.Secret],
	trafficPolicy *v1alpha1.TrafficPolicy,
	policyTargetName string,
	policyTarget *api.PolicyTarget,
	isMcpTarget bool,
) ([]AgwPolicy, error) {
	agwPolicies := make([]AgwPolicy, 0)
	var errs []error

	// Generate a base policy name from the TrafficPolicy reference
	policyName := getTrafficPolicyName(trafficPolicy.Namespace, trafficPolicy.Name, policyTargetName)

	// Convert ExtAuth policy if present
	if trafficPolicy.Spec.ExtAuth != nil && trafficPolicy.Spec.ExtAuth.ExtensionRef != nil {
		extAuthPolicies, err := processExtAuthPolicy(ctx, gatewayExtensions, trafficPolicy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing ExtAuth policy", "error", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, extAuthPolicies...)
	}

	// Convert RBAC policy if present
	if trafficPolicy.Spec.RBAC != nil {
		rbacPolicies, err := processRBACPolicy(trafficPolicy, policyName, policyTarget, isMcpTarget)
		if err != nil {
			logger.Error("error processing RBAC policy", "error", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, rbacPolicies...)
	}

	// Process AI policies if present
	if trafficPolicy.Spec.AI != nil {
		aiPolicies, err := processAIPolicy(ctx, secrets, trafficPolicy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing AI policy", "error", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, aiPolicies...)
	}
	// Process RateLimit policies if present
	if trafficPolicy.Spec.RateLimit != nil {
		rateLimitPolicies, err := processRateLimitPolicy(ctx, gatewayExtensions, trafficPolicy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing rate limit policy", "error", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, rateLimitPolicies...)
	}

	// Process transformation policies if present
	if trafficPolicy.Spec.Transformation != nil {
		transformationPolicies, err := processTransformationPolicy(trafficPolicy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing transformation policy", "error", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, transformationPolicies...)
	}

	return agwPolicies, errors.Join(errs...)
}

// processExtAuthPolicy processes ExtAuth configuration and creates corresponding agentgateway policies
func processExtAuthPolicy(ctx krt.HandlerContext, gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension], trafficPolicy *v1alpha1.TrafficPolicy, policyName string, policyTarget *api.PolicyTarget) ([]AgwPolicy, error) {
	// Look up the GatewayExtension referenced by the ExtAuth policy
	extensionName := trafficPolicy.Spec.ExtAuth.ExtensionRef.Name
	extensionNamespace := string(ptr.Deref(trafficPolicy.Spec.ExtAuth.ExtensionRef.Namespace, ""))
	if extensionNamespace == "" {
		extensionNamespace = trafficPolicy.Namespace
	}
	gwExtKey := getGatewayExtensionKey(extensionNamespace, string(extensionName))
	gwExt := krt.FetchOne(ctx, gatewayExtensions, krt.FilterKey(gwExtKey))

	if gwExt == nil || (*gwExt).Spec.Type != v1alpha1.GatewayExtensionTypeExtAuth || (*gwExt).Spec.ExtAuth == nil {
		return nil, fmt.Errorf("gateway extension not found or not of type ExtAuth: %s", gwExtKey)
	}
	extAuth := (*gwExt).Spec.ExtAuth

	// Extract service target from GatewayExtension's ExtAuth configuration
	var extauthSvcTarget *api.BackendReference
	if extAuth.GrpcService != nil && extAuth.GrpcService.BackendRef != nil {
		backendRef := extAuth.GrpcService.BackendRef
		serviceName := string(backendRef.Name)
		port := uint32(80) // default port
		if backendRef.Port != nil {
			port = uint32(*backendRef.Port) //nolint:gosec // G115: Gateway API PortNumber is always valid port range
		}
		// use trafficPolicy namespace as default
		namespace := trafficPolicy.Namespace
		if backendRef.Namespace != nil {
			namespace = string(*backendRef.Namespace)
		}
		serviceHost := kubeutils.ServiceFQDN(metav1.ObjectMeta{Namespace: namespace, Name: serviceName})
		extauthSvcTarget = &api.BackendReference{
			Kind: &api.BackendReference_Service{Service: namespace + "/" + serviceHost},
			Port: port,
		}
	}

	if extauthSvcTarget == nil {
		return nil, fmt.Errorf("failed to translate traffic policy: %s: missing extauthservice target", trafficPolicy.Name)
	}
	spec := &api.PolicySpec_ExternalAuth{
		Target:  extauthSvcTarget,
		Context: trafficPolicy.Spec.ExtAuth.ContextExtensions,
	}
	if extAuth.FailOpen {
		spec.FailureMode = api.PolicySpec_ExternalAuth_ALLOW
	} else if extAuth.StatusOnError != 0 {
		spec.FailureMode = api.PolicySpec_ExternalAuth_DENY_WITH_STATUS
		// nolint:gosec // G115: kubebuilder validation ensures safe for uint32
		spec.StatusOnError = wrapperspb.UInt32(uint32(extAuth.StatusOnError))
	}

	if b := extAuth.WithRequestBody; b != nil {
		spec.IncludeRequestBody = &api.PolicySpec_ExternalAuth_BodyOptions{
			// nolint:gosec // G115: kubebuilder validation ensures safe for uint32
			MaxRequestBytes:     uint32(b.MaxRequestBytes),
			AllowPartialMessage: b.AllowPartialMessage,
			PackAsBytes:         b.PackAsBytes,
		}
	}

	extauthPolicy := &api.Policy{
		Name:   policyName + extauthPolicySuffix + attachmentName(policyTarget),
		Target: policyTarget,
		Spec: &api.PolicySpec{
			Kind: &api.PolicySpec_ExtAuthz{
				ExtAuthz: spec,
			},
		},
	}

	logger.Debug("generated ExtAuth policy",
		"policy", trafficPolicy.Name,
		"agentgateway_policy", extauthPolicy.Name,
		"target", extauthSvcTarget)

	return []AgwPolicy{{Policy: extauthPolicy}}, nil
}

// processAIPolicy processes AI configuration and creates corresponding Agw policies
func processAIPolicy(krtctx krt.HandlerContext, secrets krt.Collection[*corev1.Secret], trafficPolicy *v1alpha1.TrafficPolicy, policyName string, policyTarget *api.PolicyTarget) ([]AgwPolicy, error) {
	var errs []error
	aiSpec := trafficPolicy.Spec.AI

	aiPolicy := &api.Policy{
		Name:   policyName + aiPolicySuffix + attachmentName(policyTarget),
		Target: policyTarget,
		Spec: &api.PolicySpec{
			Kind: &api.PolicySpec_Ai_{
				Ai: &api.PolicySpec_Ai{},
			},
		},
	}

	if aiSpec.PromptEnrichment != nil {
		aiPolicy.GetSpec().GetAi().Prompts = processPromptEnrichment(aiSpec.PromptEnrichment)
	}

	for _, def := range aiSpec.Defaults {
		val, err := toJSONValue(def.Value)
		if err != nil {
			logger.Error("error parsing field value", "field", def.Field, "error", err)
			errs = append(errs, err)
			continue
		}
		if def.Override {
			if aiPolicy.GetSpec().GetAi().Overrides == nil {
				aiPolicy.GetSpec().GetAi().Overrides = make(map[string]string)
			}
			aiPolicy.GetSpec().GetAi().Overrides[def.Field] = val
		} else {
			if aiPolicy.GetSpec().GetAi().Defaults == nil {
				aiPolicy.GetSpec().GetAi().Defaults = make(map[string]string)
			}
			aiPolicy.GetSpec().GetAi().Defaults[def.Field] = val
		}
	}

	if aiSpec.PromptGuard != nil {
		if aiPolicy.GetSpec().GetAi().PromptGuard == nil {
			aiPolicy.GetSpec().GetAi().PromptGuard = &api.PolicySpec_Ai_PromptGuard{}
		}
		if aiSpec.PromptGuard.Request != nil {
			aiPolicy.GetSpec().GetAi().PromptGuard.Request = processRequestGuard(krtctx, secrets, trafficPolicy.Namespace, aiSpec.PromptGuard.Request)
		}

		if aiSpec.PromptGuard.Response != nil {
			aiPolicy.GetSpec().GetAi().PromptGuard.Response = processResponseGuard(aiSpec.PromptGuard.Response)
		}
	}

	logger.Debug("generated AI policy",
		"policy", trafficPolicy.Name,
		"agentgateway_policy", aiPolicy.Name)

	return []AgwPolicy{{Policy: aiPolicy}}, errors.Join(errs...)
}

func processRequestGuard(krtctx krt.HandlerContext, secrets krt.Collection[*corev1.Secret], namespace string, req *v1alpha1.PromptguardRequest) *api.PolicySpec_Ai_RequestGuard {
	if req == nil {
		return nil
	}

	pgReq := &api.PolicySpec_Ai_RequestGuard{
		Webhook:          processWebhook(req.Webhook),
		Regex:            processRegex(req.Regex, req.CustomResponse),
		OpenaiModeration: processModeration(krtctx, secrets, namespace, req.Moderation),
	}

	if req.CustomResponse != nil {
		pgReq.Rejection = &api.PolicySpec_Ai_RequestRejection{
			Body:   []byte(*req.CustomResponse.Message),
			Status: uint32(*req.CustomResponse.StatusCode), // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
		}
	}

	return pgReq
}

func processResponseGuard(resp *v1alpha1.PromptguardResponse) *api.PolicySpec_Ai_ResponseGuard {
	return &api.PolicySpec_Ai_ResponseGuard{
		Webhook: processWebhook(resp.Webhook),
		Regex:   processRegex(resp.Regex, nil),
	}
}

func processPromptEnrichment(enrichment *v1alpha1.AIPromptEnrichment) *api.PolicySpec_Ai_PromptEnrichment {
	pgPromptEnrichment := &api.PolicySpec_Ai_PromptEnrichment{}

	// Add prepend messages
	for _, msg := range enrichment.Prepend {
		pgPromptEnrichment.Prepend = append(pgPromptEnrichment.Prepend, &api.PolicySpec_Ai_Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Add append messages
	for _, msg := range enrichment.Append {
		pgPromptEnrichment.Append = append(pgPromptEnrichment.Append, &api.PolicySpec_Ai_Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return pgPromptEnrichment
}

func processWebhook(webhook *v1alpha1.Webhook) *api.PolicySpec_Ai_Webhook {
	if webhook == nil {
		return nil
	}

	w := &api.PolicySpec_Ai_Webhook{
		Host: webhook.Host.Host,
		Port: uint32(webhook.Host.Port), //nolint:gosec // G115: webhook port is validated to be valid port range
	}

	if len(webhook.ForwardHeaderMatches) > 0 {
		headers := make([]*api.HeaderMatch, 0, len(webhook.ForwardHeaderMatches))
		for _, match := range webhook.ForwardHeaderMatches {
			switch ptr.Deref(match.Type, gwv1.HeaderMatchExact) {
			case gwv1.HeaderMatchExact:
				headers = append(headers, &api.HeaderMatch{
					Name:  string(match.Name),
					Value: &api.HeaderMatch_Exact{Exact: match.Value},
				})

			case gwv1.HeaderMatchRegularExpression:
				headers = append(headers, &api.HeaderMatch{
					Name:  string(match.Name),
					Value: &api.HeaderMatch_Regex{Regex: match.Value},
				})
			}
		}
		w.ForwardHeaderMatches = headers
	}

	return w
}

func processBuiltinRegexRule(builtin v1alpha1.BuiltIn, logger *slog.Logger) *api.PolicySpec_Ai_RegexRule {
	builtinValue, ok := api.PolicySpec_Ai_BuiltinRegexRule_value[string(builtin)]
	if !ok {
		logger.Warn("unknown builtin regex rule", "builtin", builtin)
		builtinValue = int32(api.PolicySpec_Ai_BUILTIN_UNSPECIFIED)
	}
	return &api.PolicySpec_Ai_RegexRule{
		Kind: &api.PolicySpec_Ai_RegexRule_Builtin{
			Builtin: api.PolicySpec_Ai_BuiltinRegexRule(builtinValue),
		},
	}
}

func processNamedRegexRule(pattern, name string) *api.PolicySpec_Ai_RegexRule {
	return &api.PolicySpec_Ai_RegexRule{
		Kind: &api.PolicySpec_Ai_RegexRule_Regex{
			Regex: &api.PolicySpec_Ai_NamedRegex{
				Pattern: pattern,
				Name:    name,
			},
		},
	}
}

func processRegex(regex *v1alpha1.Regex, customResponse *v1alpha1.CustomResponse) *api.PolicySpec_Ai_RegexRules {
	if regex == nil {
		return nil
	}

	rules := &api.PolicySpec_Ai_RegexRules{}
	if regex.Action != nil {
		rules.Action = &api.PolicySpec_Ai_Action{}
		switch *regex.Action {
		case v1alpha1.MASK:
			rules.Action.Kind = api.PolicySpec_Ai_MASK
		case v1alpha1.REJECT:
			rules.Action.Kind = api.PolicySpec_Ai_REJECT
			rules.Action.RejectResponse = &api.PolicySpec_Ai_RequestRejection{}
			if customResponse != nil {
				if customResponse.Message != nil {
					rules.Action.RejectResponse.Body = []byte(*customResponse.Message)
				}
				if customResponse.StatusCode != nil {
					rules.Action.RejectResponse.Status = uint32(*customResponse.StatusCode) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
				}
			}
		default:
			logger.Warn("unsupported regex action", "action", *regex.Action)
			rules.Action.Kind = api.PolicySpec_Ai_ACTION_UNSPECIFIED
		}
	}

	for _, match := range regex.Matches {
		// TODO(jmcguire98): should we really allow empty patterns on regex matches?
		// I see the CRD is omitempty, but I don't get why
		// for now i'm just dropping them on the floor
		if match.Pattern == nil {
			continue
		}

		// we should probably not pass an empty name to the dataplane even if none was provided,
		// since the name is what will be used for masking
		// if the action is mask
		name := ""
		if match.Name != nil {
			name = *match.Name
		}

		rules.Rules = append(rules.Rules, processNamedRegexRule(*match.Pattern, name))
	}

	for _, builtin := range regex.Builtins {
		rules.Rules = append(rules.Rules, processBuiltinRegexRule(builtin, logger))
	}

	return rules
}

func processModeration(krtctx krt.HandlerContext, secrets krt.Collection[*corev1.Secret], namespace string, moderation *v1alpha1.Moderation) *api.PolicySpec_Ai_Moderation {
	// right now we only support OpenAI moderation, so we can return nil if the moderation is nil or the OpenAIModeration is nil
	if moderation == nil || moderation.OpenAIModeration == nil {
		return nil
	}

	pgModeration := &api.PolicySpec_Ai_Moderation{}

	if moderation.OpenAIModeration.Model != nil {
		pgModeration.Model = &wrapperspb.StringValue{
			Value: *moderation.OpenAIModeration.Model,
		}
	}

	switch moderation.OpenAIModeration.AuthToken.Kind {
	case v1alpha1.Inline:
		if moderation.OpenAIModeration.AuthToken.Inline != nil {
			pgModeration.Auth = &api.BackendAuthPolicy{
				Kind: &api.BackendAuthPolicy_Key{
					Key: &api.Key{
						Secret: *moderation.OpenAIModeration.AuthToken.Inline,
					},
				},
			}
		}
	case v1alpha1.SecretRef:
		if moderation.OpenAIModeration.AuthToken.SecretRef != nil {
			// Resolve the actual secret value from Kubernetes
			secret, err := kubeutils.GetSecret(secrets, krtctx, moderation.OpenAIModeration.AuthToken.SecretRef.Name, namespace)
			if err != nil {
				logger.Error("failed to get secret for OpenAI moderation", "secret", moderation.OpenAIModeration.AuthToken.SecretRef.Name, "namespace", namespace, "error", err)
				return nil
			}

			authKey, exists := kubeutils.GetSecretAuth(secret)
			if !exists {
				logger.Error("secret does not contain valid Authorization value", "secret", moderation.OpenAIModeration.AuthToken.SecretRef.Name, "namespace", namespace)
				return nil
			}

			pgModeration.Auth = &api.BackendAuthPolicy{
				Kind: &api.BackendAuthPolicy_Key{
					Key: &api.Key{
						Secret: authKey,
					},
				},
			}
		}
	case v1alpha1.Passthrough:
		pgModeration.Auth = &api.BackendAuthPolicy{
			Kind: &api.BackendAuthPolicy_Passthrough{
				Passthrough: &api.Passthrough{},
			},
		}
	}

	return pgModeration
}

// processRBACPolicy processes RBAC configuration and creates corresponding Agw policies
func processRBACPolicy(
	trafficPolicy *v1alpha1.TrafficPolicy,
	policyName string,
	policyTarget *api.PolicyTarget,
	isMCP bool,
) ([]AgwPolicy, error) {
	var allowPolicies, denyPolicies []string
	if trafficPolicy.Spec.RBAC.Action == v1alpha1.AuthorizationPolicyActionDeny {
		denyPolicies = append(denyPolicies, trafficPolicy.Spec.RBAC.Policy.MatchExpressions...)
	} else {
		allowPolicies = append(allowPolicies, trafficPolicy.Spec.RBAC.Policy.MatchExpressions...)
	}

	var rbacPolicy *api.Policy
	if isMCP {
		rbacPolicy = &api.Policy{
			Name:   policyName + rbacPolicySuffix + attachmentName(policyTarget),
			Target: policyTarget,
			Spec: &api.PolicySpec{
				Kind: &api.PolicySpec_McpAuthorization{
					McpAuthorization: &api.PolicySpec_RBAC{
						Allow: allowPolicies,
						Deny:  denyPolicies,
					},
				},
			},
		}
	} else {
		rbacPolicy = &api.Policy{
			Name:   policyName + rbacPolicySuffix + attachmentName(policyTarget),
			Target: policyTarget,
			Spec: &api.PolicySpec{
				Kind: &api.PolicySpec_Authorization{
					Authorization: &api.PolicySpec_RBAC{
						Allow: allowPolicies,
						Deny:  denyPolicies,
					},
				},
			},
		}
	}

	logger.Debug("generated RBAC policy",
		"policy", trafficPolicy.Name,
		"agentgateway_policy", rbacPolicy.Name,
		"target", policyTarget)

	return []AgwPolicy{{Policy: rbacPolicy}}, nil
}

func getTrafficPolicyName(trafficPolicyNs, trafficPolicyName, policyTargetName string) string {
	return fmt.Sprintf("trafficpolicy/%s/%s/%s", trafficPolicyNs, trafficPolicyName, policyTargetName)
}

func getBackendKey(targetPolicyNs, targetName string) string {
	return fmt.Sprintf("%s/%s", targetPolicyNs, targetName)
}

func getGatewayExtensionKey(extensionNamespace, extensionName string) string {
	return fmt.Sprintf("%s/%s", extensionNamespace, extensionName)
}

// processRateLimitPolicy processes RateLimit configuration and creates corresponding agentgateway policies
func processRateLimitPolicy(ctx krt.HandlerContext, gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension], trafficPolicy *v1alpha1.TrafficPolicy, policyName string, policyTarget *api.PolicyTarget) ([]AgwPolicy, error) {
	var agwPolicies []AgwPolicy

	// Process local rate limiting if present
	if trafficPolicy.Spec.RateLimit.Local != nil {
		localPolicy, err := processLocalRateLimitPolicy(trafficPolicy, policyName, policyTarget)
		if localPolicy != nil && err == nil {
			agwPolicies = append(agwPolicies, *localPolicy)
		} else {
			return nil, err
		}
	}

	// Process global rate limiting if present
	if trafficPolicy.Spec.RateLimit.Global != nil {
		globalPolicy, err := processGlobalRateLimitPolicy(ctx, gatewayExtensions, trafficPolicy, policyName, policyTarget)
		if globalPolicy != nil && err == nil {
			agwPolicies = append(agwPolicies, *globalPolicy)
		} else {
			return nil, err
		}
	}

	return agwPolicies, nil
}

// processLocalRateLimitPolicy processes local rate limiting configuration
func processLocalRateLimitPolicy(trafficPolicy *v1alpha1.TrafficPolicy, policyName string, policyTarget *api.PolicyTarget) (*AgwPolicy, error) {
	if trafficPolicy.Spec.RateLimit.Local.TokenBucket == nil {
		logger.Error("token bucket configuration is nil")
		return nil, errors.New("token bucket configuration is nil")
	}

	tokenBucket := trafficPolicy.Spec.RateLimit.Local.TokenBucket

	// Validate configuration
	if tokenBucket.MaxTokens <= 0 {
		return nil, fmt.Errorf("invalid max tokens value: %d", tokenBucket.MaxTokens)
	}
	// Convert duration to seconds for agentgateway
	fillIntervalSeconds := uint32(tokenBucket.FillInterval.Duration.Seconds())
	if fillIntervalSeconds == 0 {
		fillIntervalSeconds = 1 // minimum 1 second
	}

	// Create local rate limit policy using the proper agentgateway API
	tokensPerFill := uint64(ptr.Deref(tokenBucket.TokensPerFill, 1)) // nolint:gosec // G115: kubebuilder validation ensures safe for uint64
	if tokensPerFill == 0 {
		tokensPerFill = 1
	}

	localRateLimitPolicy := &api.Policy{
		Name:   policyName + localRateLimitPolicySuffix + attachmentName(policyTarget),
		Target: policyTarget,
		Spec: &api.PolicySpec{
			Kind: &api.PolicySpec_LocalRateLimit_{
				LocalRateLimit: &api.PolicySpec_LocalRateLimit{
					MaxTokens:     uint64(tokenBucket.MaxTokens),
					TokensPerFill: tokensPerFill,
					FillInterval:  &durationpb.Duration{Seconds: int64(fillIntervalSeconds)},
					Type:          api.PolicySpec_LocalRateLimit_REQUEST,
				},
			},
		},
	}

	return &AgwPolicy{Policy: localRateLimitPolicy}, nil
}

func processGlobalRateLimitPolicy(
	ctx krt.HandlerContext,
	gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension],
	trafficPolicy *v1alpha1.TrafficPolicy,
	policyName string,
	policyTarget *api.PolicyTarget,
) (*AgwPolicy, error) {
	grl := trafficPolicy.Spec.RateLimit.Global
	if grl == nil {
		return nil, nil
	}

	gwExt, err := lookupGatewayExtension(
		ctx, gatewayExtensions, grl.ExtensionRef, trafficPolicy.Namespace, v1alpha1.GatewayExtensionTypeRateLimit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup rate limit extension: %w", err)
	}
	if gwExt.Spec.RateLimit == nil ||
		gwExt.Spec.RateLimit.GrpcService == nil ||
		gwExt.Spec.RateLimit.GrpcService.BackendRef == nil {
		return nil, fmt.Errorf("rate limit extension missing grpcService.backendRef: %s", gwExt.Name)
	}

	// Build BackendReference for agentgateway (service/ns + port)
	agwRef, err := buildAGWServiceRef(gwExt.Spec.RateLimit.GrpcService.BackendRef, trafficPolicy.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to build AGW service reference: %w", err)
	}

	// Translate descriptors
	descriptors := make([]*api.PolicySpec_RemoteRateLimit_Descriptor, 0, len(grl.Descriptors))
	for _, d := range grl.Descriptors {
		if agw := processRateLimitDescriptor(d); agw != nil {
			descriptors = append(descriptors, agw)
		}
	}

	// Build the RemoteRateLimit policy that agentgateway expects
	p := &api.Policy{
		Name:   policyName + globalRateLimitPolicySuffix + attachmentName(policyTarget),
		Target: policyTarget,
		Spec: &api.PolicySpec{
			Kind: &api.PolicySpec_RemoteRateLimit_{
				RemoteRateLimit: &api.PolicySpec_RemoteRateLimit{
					Domain:      gwExt.Spec.RateLimit.Domain,
					Target:      agwRef,
					Descriptors: descriptors,
				},
			},
		},
	}

	return &AgwPolicy{Policy: p}, nil
}

// getDescriptorEntryKey extracts the key from a rate limit descriptor entry
func getDescriptorEntryKey(entry v1alpha1.RateLimitDescriptorEntry) string {
	switch entry.Type {
	case v1alpha1.RateLimitDescriptorEntryTypeGeneric:
		if entry.Generic != nil {
			return entry.Generic.Key
		}
	case v1alpha1.RateLimitDescriptorEntryTypeHeader:
		if entry.Header != nil {
			return *entry.Header
		}
	case v1alpha1.RateLimitDescriptorEntryTypeRemoteAddress:
		return "remote_address"
	case v1alpha1.RateLimitDescriptorEntryTypePath:
		return "path"
	}
	return ""
}

func lookupGatewayExtension(ctx krt.HandlerContext, gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension], extensionRef v1alpha1.NamespacedObjectReference, defaultNamespace string, expectedType v1alpha1.GatewayExtensionType) (*v1alpha1.GatewayExtension, error) {
	extensionName := extensionRef.Name
	extensionNamespace := string(ptr.Deref(extensionRef.Namespace, ""))
	if extensionNamespace == "" {
		extensionNamespace = defaultNamespace
	}

	gwExtKey := fmt.Sprintf("%s/%s", extensionNamespace, extensionName)
	gwExt := krt.FetchOne(ctx, gatewayExtensions, krt.FilterKey(gwExtKey))

	if gwExt == nil {
		return nil, fmt.Errorf("gateway extension not found: %s", gwExtKey)
	}

	if (*gwExt).Spec.Type != expectedType {
		return nil, fmt.Errorf("gateway extension is not of type %s: %s", expectedType, gwExtKey)
	}

	return *gwExt, nil
}

func buildServiceHost(backendRef *gwv1.BackendRef, defaultNamespace string) (serviceName, namespace string, port uint32, err error) {
	if backendRef == nil {
		return "", "", 0, fmt.Errorf("backend reference is nil")
	}

	serviceName = string(backendRef.Name)
	if serviceName == "" {
		return "", "", 0, fmt.Errorf("service name is empty")
	}

	port = uint32(80) // default port
	if backendRef.Port != nil {
		port = uint32(*backendRef.Port) //nolint:gosec // G115: Gateway API PortNumber is always valid port range
	}

	namespace = defaultNamespace
	if backendRef.Namespace != nil {
		namespace = string(*backendRef.Namespace)
	}
	return serviceName, namespace, port, nil
}

func processRateLimitDescriptor(descriptor v1alpha1.RateLimitDescriptor) *api.PolicySpec_RemoteRateLimit_Descriptor {
	if len(descriptor.Entries) == 0 {
		return nil
	}

	entries := make([]*api.PolicySpec_RemoteRateLimit_Entry, 0, len(descriptor.Entries))

	for _, entry := range descriptor.Entries {
		// Map TrafficPolicy entry -> CEL expression or literal expected by the agent.
		var value string
		key := getDescriptorEntryKey(entry)
		switch entry.Type {
		case v1alpha1.RateLimitDescriptorEntryTypeGeneric:
			if entry.Generic != nil {
				// Constant literal -> quote for CEL
				value = strconv.Quote(entry.Generic.Value)
			}
		case v1alpha1.RateLimitDescriptorEntryTypeHeader:
			if entry.Header != nil {
				// Dynamic: header value expression
				value = celHeaderExpr(*entry.Header)
			}
		case v1alpha1.RateLimitDescriptorEntryTypeRemoteAddress:
			value = celRemoteIPExpr()
		case v1alpha1.RateLimitDescriptorEntryTypePath:
			value = celPathExpr()
		}
		if key != "" && value != "" {
			entries = append(entries, &api.PolicySpec_RemoteRateLimit_Entry{
				Key:   key,
				Value: value,
			})
		}
	}

	if len(entries) == 0 {
		return nil
	}
	return &api.PolicySpec_RemoteRateLimit_Descriptor{
		Entries: entries,
		Type:    api.PolicySpec_RemoteRateLimit_REQUESTS,
	}
}

// celHeaderExpr returns a CEL expression that reads a request header.
func celHeaderExpr(name string) string {
	// Convert to lowercase to match how HTTP headers are stored
	return fmt.Sprintf(`request.headers[%q]`, strings.ToLower(name))
}

// celRemoteIPExpr returns a CEL expression for the client IP.
func celRemoteIPExpr() string {
	return "source.address"
}

// celPathExpr returns a CEL expression for the request path (if supported in your env).
func celPathExpr() string {
	return "request.path"
}

// Returns an agentgateway BackendReference.Service in the "<ns>/<fqdn>" form + Port.
func buildAGWServiceRef(br *gwv1.BackendRef, defaultNS string) (*api.BackendReference, error) {
	if br == nil {
		return nil, fmt.Errorf("backendRef is nil")
	}
	name, ns, port, err := buildServiceHost(br, defaultNS)
	if err != nil {
		return nil, err
	}
	fqdn := kubeutils.ServiceFQDN(metav1.ObjectMeta{Namespace: ns, Name: name})
	return &api.BackendReference{
		Kind: &api.BackendReference_Service{Service: ns + "/" + fqdn},
		Port: port,
	}, nil
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

// processTransformationPolicy processes transformation configuration and creates corresponding Agw policies
func processTransformationPolicy(
	trafficPolicy *v1alpha1.TrafficPolicy,
	policyName string,
	policyTarget *api.PolicyTarget,
) ([]AgwPolicy, error) {
	var errs []error
	transformation := trafficPolicy.Spec.Transformation

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
			Spec: &api.PolicySpec{
				Kind: &api.PolicySpec_Transformation{
					Transformation: &api.PolicySpec_TransformationPolicy{
						Request:  convertedReq,
						Response: convertedResp,
					},
				},
			},
		}

		logger.Debug("generated transformation policy",
			"policy", trafficPolicy.Name,
			"agentgateway_policy", transformationPolicy.Name,
			"target", policyTarget)
		return []AgwPolicy{{Policy: transformationPolicy}}, errors.Join(errs...)
	}
	return nil, errors.Join(errs...)
}

// convertTransformSpec converts transformation specs to agentgateway format
func convertTransformSpec(spec *v1alpha1.Transform) (*api.PolicySpec_TransformationPolicy_Transform, error) {
	if spec == nil {
		return nil, nil
	}

	var errs []error
	var transform *api.PolicySpec_TransformationPolicy_Transform

	for _, header := range spec.Set {
		headerValue := header.Value
		if isCEL(headerValue) {
			if transform == nil {
				transform = &api.PolicySpec_TransformationPolicy_Transform{}
			}
			transform.Set = append(transform.Set, &api.PolicySpec_HeaderTransformation{
				Name:       string(header.Name),
				Expression: string(header.Value),
			})
		} else {
			errs = append(errs, fmt.Errorf("invalid header value: %s", headerValue))
		}
	}

	for _, header := range spec.Add {
		headerValue := header.Value
		if isCEL(headerValue) {
			if transform == nil {
				transform = &api.PolicySpec_TransformationPolicy_Transform{}
			}
			transform.Add = append(transform.Add, &api.PolicySpec_HeaderTransformation{
				Name:       string(header.Name),
				Expression: string(header.Value),
			})
		} else {
			errs = append(errs, fmt.Errorf("invalid header value: %s", headerValue))
		}
	}

	if spec.Remove != nil {
		if transform == nil {
			transform = &api.PolicySpec_TransformationPolicy_Transform{}
		}
		transform.Remove = spec.Remove
	}

	if spec.Body != nil {
		// Warn if ParseAs is set since it's not supported for agentgateway
		// default is set to AsString
		if spec.Body.ParseAs == v1alpha1.BodyParseBehaviorAsJSON {
			logger.Warn("parseAs field is ignored for agentgateway, use json() function directly in CEL expressions",
				"parse_as", spec.Body.ParseAs)
		}

		// Handle body transformation if present
		if spec.Body.Value != nil {
			bodyValue := *spec.Body.Value
			if isCEL(bodyValue) {
				if transform == nil {
					transform = &api.PolicySpec_TransformationPolicy_Transform{}
				}
				transform.Body = &api.PolicySpec_BodyTransformation{
					Expression: string(bodyValue),
				}
			} else {
				errs = append(errs, fmt.Errorf("invalid body value: %s", bodyValue))
			}
		}
	}

	return transform, errors.Join(errs...)
}

// Checks if the expression is a valid CEL expression
func isCEL(expr v1alpha1.Template) bool {
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
