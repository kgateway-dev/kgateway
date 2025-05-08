package waypoint

import (
	"fmt"

	"github.com/caarlos0/log"
	"google.golang.org/protobuf/types/known/anypb"
	authpb "istio.io/api/security/v1"
	authcr "istio.io/client-go/pkg/apis/security/v1"
	"istio.io/istio/pilot/pkg/config/kube/crdclient"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pilot/pkg/security/authz/builder"
	"istio.io/istio/pilot/pkg/security/trustdomain"
	"istio.io/istio/pkg/config/schema/gvk"
	gwapi "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/waypoint/waypointquery"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/filters"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

const (
	// TODO: Add configuration for trustDomain and trustDomainAliases in settings
	// This will allow users to customize the trust domain and its aliases for their cluster
	defaultTrustDomain = "cluster.local"
)

// BuildRBAC uses whatever policies received, assumes that the policy attachment is done outside
// gives the following lists of filters:
// tcpRBAC - only used in tcp chains (using this on an HTTP chain could cause improper DENY)
// httpRBAC - only used in http chains
func BuildRBAC(
	authzPolicies []*authcr.AuthorizationPolicy,
	gw *gwapi.Gateway,
	svc *waypointquery.Service,
) (
	tcpRBAC []*ir.CustomEnvoyFilter,
	httpRBAC []*ir.CustomEnvoyFilter,
) {
	// Deduplicate and separate policies by action
	policyResult := separateAndDeduplicatePolicies(authzPolicies)

	// If no policies are applicable, return early
	if len(policyResult.Deny) == 0 && len(policyResult.Allow) == 0 &&
		len(policyResult.Audit) == 0 && len(policyResult.Custom) == 0 {
		return nil, nil
	}

	// Create the builder with our separated policies
	trustBundle := trustdomain.NewBundle(defaultTrustDomain, nil)
	authzBuilder := builder.New(trustBundle, nil, policyResult, builder.Option{
		IsCustomBuilder: false,
		UseFilterState:  true,
	})

	const stage = filters.FilterStage_AuthZStage
	const predicate = filters.FilterStage_After

	tcpFilters := authzBuilder.BuildTCP()
	httpFilters := authzBuilder.BuildHTTP()

	if len(tcpFilters) > 0 {
		tcpRBAC = append(tcpRBAC, CustomNetworkFilters(tcpFilters, stage, predicate)...)
	}
	if len(httpFilters) > 0 {
		httpRBAC = append(httpRBAC, CustomHTTPFilters(httpFilters, stage, predicate)...)
	}
	return tcpRBAC, httpRBAC
}

func applyHTTPRBACFilters(httpChain *ir.HttpFilterChainIR, httpRBAC []*ir.CustomEnvoyFilter) {
	// Apply RBAC filters regardless of the presence of proxy_protocol_authority
	if len(httpRBAC) > 0 {
		// Initialize CustomHTTPFilters if it's nil
		if httpChain.CustomHTTPFilters == nil {
			httpChain.CustomHTTPFilters = []ir.CustomEnvoyFilter{}
		}

		// Add RBAC filters to CustomHTTPFilters
		for _, f := range httpRBAC {
			httpChain.CustomHTTPFilters = append(httpChain.CustomHTTPFilters, *f)
		}
	}
}

func applyTCPRBACFilters(tcpChain *ir.TcpIR, tcpRBAC []*ir.CustomEnvoyFilter, svc waypointquery.Service) {
	// Apply RBAC filters regardless of the presence of proxy_protocol_authority
	if len(tcpRBAC) > 0 {
		if tcpChain.NetworkFilters == nil {
			tcpChain.NetworkFilters = []*anypb.Any{}
		}

		// Add RBAC filters as built-in network filters
		for _, f := range tcpRBAC {
			tcpChain.NetworkFilters = append(tcpChain.NetworkFilters, f.Config)
		}
	}
}

// separateAndDeduplicatePolicies takes a list of policies and returns them
// separated by action type with duplicates removed
func separateAndDeduplicatePolicies(policies []*authcr.AuthorizationPolicy) model.AuthorizationPoliciesResult {
	// Use a map to track seen policies to avoid duplicates
	seen := make(map[string]bool)
	result := model.AuthorizationPoliciesResult{}

	for _, policy := range policies {
		// Create a unique key for this policy
		key := fmt.Sprintf("%s/%s", policy.GetNamespace(), policy.GetName())

		// Skip if we've already processed this policy
		if seen[key] {
			continue
		}
		seen[key] = true

		// Convert to Istio model type
		convertedSpec := crdclient.TranslateObject(policy, gvk.AuthorizationPolicy, "").Spec.(*authpb.AuthorizationPolicy)
		convertedPolicy := model.AuthorizationPolicy{
			Name:        policy.Name,
			Namespace:   policy.Namespace,
			Annotations: map[string]string{},
			Spec:        convertedSpec,
		}

		// Add to the appropriate slice based on action
		switch convertedSpec.GetAction() {
		case authpb.AuthorizationPolicy_ALLOW:
			result.Allow = append(result.Allow, convertedPolicy)
		case authpb.AuthorizationPolicy_DENY:
			result.Deny = append(result.Deny, convertedPolicy)
		case authpb.AuthorizationPolicy_AUDIT:
			result.Audit = append(result.Audit, convertedPolicy)
		case authpb.AuthorizationPolicy_CUSTOM:
			result.Custom = append(result.Custom, convertedPolicy)
		default:
			// Log error for unsupported action
			log.Errorf("ignored authorization policy %s.%s with unsupported action: %s",
				policy.GetNamespace(), policy.GetName(), convertedSpec.GetAction())
		}
	}

	return result
}
