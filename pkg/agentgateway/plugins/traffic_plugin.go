package plugins

import (
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

const (
	extauthPolicySuffix = ":extauth"
)

// NewTrafficPlugin creates a new TrafficPolicy plugin
func NewTrafficPlugin(agw *AgwCollections) AgentgatewayPlugin {
	col := krt.WrapClient(kclient.NewFiltered[*v1alpha1.TrafficPolicy](
		agw.Client,
		kclient.Filter{ObjectFilter: agw.Client.ObjectFilter()},
	), agw.KrtOpts.ToOptions("TrafficPolicy")...)
	policyCol := krt.NewManyCollection(col, func(krtctx krt.HandlerContext, policyCR *v1alpha1.TrafficPolicy) []ADPPolicy {
		return translateTrafficPolicy(krtctx, agw.GatewayExtensions, policyCR)
	})

	return AgentgatewayPlugin{
		ContributesPolicies: map[schema.GroupKind]PolicyPlugin{
			wellknown.TrafficPolicyGVK.GroupKind(): {
				Policies: policyCol,
			},
		},
		ExtraHasSynced: func() bool {
			return policyCol.HasSynced()
		},
	}
}

// translateTrafficPolicy generates policies for a single traffic policy
func translateTrafficPolicy(ctx krt.HandlerContext, gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension], trafficPolicy *v1alpha1.TrafficPolicy) []ADPPolicy {
	logger := logging.New("agentgateway/plugins/traffic")
	var adpPolicies []ADPPolicy

	for _, target := range trafficPolicy.Spec.TargetRefs {
		var policyTarget *api.PolicyTarget

		switch string(target.Kind) {
		case wellknown.GatewayKind:
			policyTarget = &api.PolicyTarget{
				Kind: &api.PolicyTarget_Gateway{
					Gateway: trafficPolicy.Namespace + "/" + string(target.Name),
				},
			}
			// TODO(npolshak): add listener support once https://github.com/agentgateway/agentgateway/pull/323 goes in
			//if target.SectionName != nil {
			//	policyTarget = &api.PolicyTarget{
			//		Kind: &api.PolicyTarget_Listener{
			//			Listener: InternalGatewayName(trafficPolicy.Namespace, string(target.Name), string(*target.SectionName)),
			//		},
			//	}
			//}

		case wellknown.HTTPRouteKind:
			policyTarget = &api.PolicyTarget{
				Kind: &api.PolicyTarget_Route{
					Route: trafficPolicy.Namespace + "/" + string(target.Name),
				},
			}
			// TODO(npolshak): add route rule support once https://github.com/agentgateway/agentgateway/pull/323 goes in
			//if target.SectionName != nil {
			//	policyTarget = &api.PolicyTarget{
			//		Kind: &api.PolicyTarget_RouteRule{
			//			RouteRule: trafficPolicy.Namespace + "/" + string(target.Name) + "/" + string(*target.SectionName),
			//		},
			//	}
			//}

		default:
			logger.Warn("unsupported target kind", "kind", target.Kind, "policy", trafficPolicy.Name)
			continue
		}

		if policyTarget != nil {
			translatedPolicies := translateTrafficPolicyToADP(ctx, gatewayExtensions, trafficPolicy, string(target.Name), policyTarget)
			adpPolicies = append(adpPolicies, translatedPolicies...)
		}
	}

	return adpPolicies
}

// translateTrafficPolicyToADP converts a TrafficPolicy to agentgateway Policy resources
func translateTrafficPolicyToADP(ctx krt.HandlerContext, gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension], trafficPolicy *v1alpha1.TrafficPolicy, policyTargetName string, policyTarget *api.PolicyTarget) []ADPPolicy {
	adpPolicies := make([]ADPPolicy, 0)

	// Generate a base policy name from the TrafficPolicy reference
	policyName := fmt.Sprintf("trafficpolicy/%s/%s/%s", trafficPolicy.Namespace, trafficPolicy.Name, policyTargetName)

	// Convert ExtAuth policy if present
	if trafficPolicy.Spec.ExtAuth != nil && trafficPolicy.Spec.ExtAuth.ExtensionRef.Name != "" {
		extAuthPolicies := processExtAuthPolicy(ctx, gatewayExtensions, trafficPolicy, policyName, policyTarget)
		adpPolicies = append(adpPolicies, extAuthPolicies...)
	}

	// TODO: Add support for other policy types as needed:
	// - RateLimit
	// - Transformation
	// - ExtProc
	// - AI policies
	// etc.

	return adpPolicies
}

// processExtAuthPolicy processes ExtAuth configuration and creates corresponding agentgateway policies
func processExtAuthPolicy(ctx krt.HandlerContext, gatewayExtensions krt.Collection[*v1alpha1.GatewayExtension], trafficPolicy *v1alpha1.TrafficPolicy, policyName string, policyTarget *api.PolicyTarget) []ADPPolicy {
	logger := logging.New("agentgateway/plugins/traffic")

	// Look up the GatewayExtension referenced by the ExtAuth policy
	extensionName := trafficPolicy.Spec.ExtAuth.ExtensionRef.Name
	extensionNamespace := string(ptr.Deref(trafficPolicy.Spec.ExtAuth.ExtensionRef.Namespace, ""))
	if extensionNamespace == "" {
		extensionNamespace = trafficPolicy.Namespace
	}
	gwExtKey := fmt.Sprintf("%s/%s", extensionNamespace, extensionName)
	gwExt := krt.FetchOne(ctx, gatewayExtensions, krt.FilterKey(gwExtKey))

	if gwExt == nil || (*gwExt).Spec.Type != v1alpha1.GatewayExtensionTypeExtAuth || (*gwExt).Spec.ExtAuth == nil {
		logger.Error("gateway extension not found or not of type ExtAuth", "extension", gwExtKey)
		return nil
	}
	extAuth := (*gwExt).Spec.ExtAuth

	// Extract service target from GatewayExtension's ExtAuth configuration
	var extauthSvcTarget *api.BackendReference
	if extAuth.GrpcService != nil && extAuth.GrpcService.BackendRef != nil {
		backendRef := extAuth.GrpcService.BackendRef
		serviceName := string(backendRef.Name)
		port := uint32(80) // default port
		if backendRef.Port != nil {
			port = uint32(*backendRef.Port)
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
		logger.Warn("failed to translate traffic policy", "policy", trafficPolicy.Name, "target", policyTarget, "error", "missing extauthservice target")
		return nil
	}

	extauthPolicy := &api.Policy{
		Name:   policyName + extauthPolicySuffix,
		Target: policyTarget,
		Spec: &api.PolicySpec{
			Kind: &api.PolicySpec_ExtAuthz{
				ExtAuthz: &api.PolicySpec_ExternalAuth{
					Target:  extauthSvcTarget,
					Context: trafficPolicy.Spec.ExtAuth.ContextExtensions,
				},
			},
		},
	}

	logger.Debug("generated ExtAuth policy",
		"policy", trafficPolicy.Name,
		"agentgateway_policy", extauthPolicy.Name,
		"target", extauthSvcTarget)

	return []ADPPolicy{{Policy: extauthPolicy}}
}
