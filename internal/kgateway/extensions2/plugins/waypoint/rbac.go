package waypoint

import (
	"google.golang.org/protobuf/types/known/anypb"
	"istio.io/api/label"
	authpb "istio.io/api/security/v1"
	authcr "istio.io/client-go/pkg/apis/security/v1"
	"istio.io/istio/pilot/pkg/config/kube/crdclient"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pilot/pkg/security/authz/builder"
	"istio.io/istio/pilot/pkg/security/trustdomain"
	"istio.io/istio/pkg/config/schema/gvk"
	gwapi "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/waypoint/waypointquery"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/filters"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

const (
	// TODO: Add configuration for trustDomain and trustDomainAliases in settings
	// This will allow users to customize the trust domain and its aliases for their cluster
	defaultTrustDomain = "cluster.local"
)

var (
	// RootNamespace is the namespace where Istio control plane components are installed.
	// It is set during initialization via SetRootNamespace() which reads from settings.IstioNamespace.
	// The default value is "istio-system" if not configured.
	RootNamespace = "istio-system"
)

// SetRootNamespace sets the RootNamespace from settings.
// This should be called during initialization.
func SetRootNamespace(s *settings.Settings) {
	if s != nil {
		RootNamespace = s.IstioNamespace
	}
}

// BuildRBACForService gives three lists of filters:
// tcpRBAC - only used in tcp chains (using this on an HTTP chain could cause improper DENY)
// httpRBAC - only used in http chains
// that passes id from metadata to filter state (see ProxyProtocolTLVAuthorityNetworkFilter)
func BuildRBACForService(
	authzPolicies []*authcr.AuthorizationPolicy,
	gw *gwapi.Gateway,
	svc *waypointquery.Service,
) (
	tcpRBAC []*ir.CustomEnvoyFilter,
	httpRBAC []*ir.CustomEnvoyFilter,
) {
	authzBuilder := getAuthzBuilder(authzPolicies, gw.Name, gw.Namespace, RootNamespace, svc)
	if authzBuilder != nil {
		const stage = filters.FilterStage_AuthZStage
		const predicate = filters.FilterStage_After

		tcpFilters := authzBuilder.BuildTCP()
		httpFilters := authzBuilder.BuildHTTP()

		if len(tcpFilters) > 0 {
			tcpRBAC = append(tcpRBAC, ir.CustomNetworkFilters(tcpFilters, stage, predicate)...)
		}
		if len(httpFilters) > 0 {
			httpRBAC = ir.CustomHTTPFilters(httpFilters, stage, predicate)
		}
	}
	return tcpRBAC, httpRBAC
}

// getAuthzBuilder constructs the istio builder.
// It can be nil if it filters out all the policies.
// This relies heavily on Istio code so that we can get similar behavior:
// https://github.com/istio/istio/blob/master/pilot/pkg/model/policyattachment.go
func getAuthzBuilder(
	policies []*authcr.AuthorizationPolicy,
	gatewayName, gatewayNamespace string,
	rootNamespace string,
	svc *waypointquery.Service,
) *builder.Builder {
	policiesMap := model.AuthorizationPolicies{
		NamespaceToPolicies: map[string][]model.AuthorizationPolicy{},
		RootNamespace:       rootNamespace,
	}

	for _, policy := range policies {
		convertedSpec := crdclient.TranslateObject(policy, gvk.AuthorizationPolicy, "").Spec.(*authpb.AuthorizationPolicy)
		convertedPolicy := model.AuthorizationPolicy{
			Name:        policy.Name,
			Namespace:   policy.Namespace,
			Annotations: map[string]string{},
			Spec:        convertedSpec,
		}
		policiesMap.NamespaceToPolicies[policy.Namespace] = append(policiesMap.NamespaceToPolicies[policy.Namespace], convertedPolicy)
	}

	matcher := model.WorkloadPolicyMatcher{
		IsWaypoint: true,
		Services: []model.ServiceInfoForPolicyMatcher{
			{
				Name:      svc.GetName(),
				Namespace: svc.GetNamespace(),
				Registry:  svc.Provider(),
			},
		},
		WorkloadNamespace: gatewayNamespace,
		WorkloadLabels: map[string]string{
			label.IoK8sNetworkingGatewayGatewayName.Name: gatewayName,
		},
	}

	// Call the function
	policyResult := policiesMap.ListAuthorizationPolicies(matcher)

	if len(policyResult.Deny) == 0 && len(policyResult.Allow) == 0 &&
		len(policyResult.Audit) == 0 && len(policyResult.Custom) == 0 {
		return nil
	}

	trustBundle := trustdomain.NewBundle(defaultTrustDomain, nil)
	builder := builder.New(trustBundle, nil, policyResult, builder.Option{
		IsCustomBuilder: false,
		UseFilterState:  true,
	})

	return builder
}

func applyHTTPRBACFilters(httpChain *ir.HttpFilterChainIR, httpRBAC []*ir.CustomEnvoyFilter, svc waypointquery.Service) {
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
