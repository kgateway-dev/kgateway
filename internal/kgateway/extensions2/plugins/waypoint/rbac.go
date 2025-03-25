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
	"istio.io/istio/pilot/pkg/serviceregistry/provider"
	"istio.io/istio/pkg/config/schema/gvk"
	gwapi "sigs.k8s.io/gateway-api/apis/v1"

	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/waypoint/waypointquery"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/filters"

	"log"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
)

const (
	// TODO RootNamespace should be equiv to what istio sees as root ns
	RootNamespace = "istio-system"
)

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
	log.Printf("BuildRBACForService called with %d policies for service %s/%s",
		len(authzPolicies), svc.GetNamespace(), svc.GetName())

	authzBuilder := getAuthzBuilder(authzPolicies, gw.Name, gw.Namespace, RootNamespace, svc)
	if authzBuilder != nil {
		const stage = filters.FilterStage_AuthZStage
		const predicate = filters.FilterStage_After

		tcpFilters := authzBuilder.BuildTCP()
		httpFilters := authzBuilder.BuildHTTP()
		// After the line that has: info  Built TCP filters: 1, HTTP filters: 1
		for i, filter := range httpFilters {
			if filter != nil {
				typedConfig := filter.GetTypedConfig()
				if typedConfig != nil {
					log.Printf("HTTP filter %d: name=%s, type_url=%s", i, filter.GetName(), typedConfig.GetTypeUrl())

					// Try to log the raw config bytes in a readable format
					rawConfig := typedConfig.GetValue()
					if len(rawConfig) > 0 {
						log.Printf("HTTP filter %d raw config (hex): %x", i, rawConfig)
						log.Printf("HTTP filter %d raw config length: %d bytes", i, len(rawConfig))
					}
				} else {
					log.Printf("HTTP filter %d: name=%s, NO TYPED CONFIG", i, filter.GetName())
				}
			} else {
				log.Printf("HTTP filter %d is nil", i)
			}
		}

		log.Printf("Built TCP filters: %d, HTTP filters: %d", len(tcpFilters), len(httpFilters))

		if len(tcpFilters) > 0 {
			tcpRBAC = append(tcpRBAC, CustomNetworkFilters(tcpFilters, stage, predicate)...)
		}
		if len(httpFilters) > 0 {
			httpRBAC = CustomHTTPFilters(httpFilters, stage, predicate)
		}
	} else {
		log.Printf("authzBuilder is nil, no RBAC filters will be generated.")
	}

	log.Printf("BuildRBACForService returning - TCP filters: %d, HTTP filters: %d", len(tcpRBAC), len(httpRBAC))

	return
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
	// Add detailed logging just before calling ListAuthorizationPolicies
	// Log all input parameters in detail
	log.Printf("DEBUG: === ListAuthorizationPolicies Input Parameters ===")
	log.Printf("DEBUG: IsWaypoint: %v", true)
	log.Printf("DEBUG: Service details:")
	log.Printf("DEBUG:   - Name: %s", svc.GetName())
	log.Printf("DEBUG:   - Namespace: %s", svc.GetNamespace())
	log.Printf("DEBUG: WorkloadNamespace: %s", gatewayNamespace)
	log.Printf("DEBUG: WorkloadLabels: %v", map[string]string{
		label.IoK8sNetworkingGatewayGatewayName.Name: gatewayName,
	})
	log.Printf("getAuthzBuilder called with gateway: %s/%s, service: %s/%s",
		gatewayNamespace, gatewayName, svc.GetNamespace(), svc.GetName())

	policiesMap := model.AuthorizationPolicies{
		NamespaceToPolicies: map[string][]model.AuthorizationPolicy{},
		RootNamespace:       rootNamespace,
	}

	// Capture the gateway name for reference
	log.Printf("DEBUG: Gateway being referenced: %s/%s", gatewayNamespace, gatewayName)

	// Log any relevant service accounts
	if svcAccount, ok := svc.GetLabels()["service.istio.io/canonical-name"]; ok {
		log.Printf("DEBUG: Service has canonical name: %s", svcAccount)
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
		log.Printf("Converted policy: namespace=%s, name=%s, spec=%+v",
			policy.Namespace, policy.Name, convertedSpec)
	}

	matcher := model.WorkloadPolicyMatcher{
		IsWaypoint: true,
		Services: []model.ServiceInfoForPolicyMatcher{
			{
				Name:      svc.GetName(),
				Namespace: svc.GetNamespace(),
				Registry:  provider.Kubernetes,
			},
		},
		WorkloadNamespace: gatewayNamespace,
		WorkloadLabels: map[string]string{
			label.IoK8sNetworkingGatewayGatewayName.Name: gatewayName,
		},
	}

	// Log the input parameters clearly
	log.Printf("DEBUG: ListAuthorizationPolicies input: IsWaypoint=%v, Service=%s/%s, WorkloadNS=%s",
		matcher.IsWaypoint, svc.GetNamespace(), svc.GetName(), gatewayNamespace)

	// Call the function
	policyResult := policiesMap.ListAuthorizationPolicies(matcher)

	// Log the detailed results
	log.Printf("DEBUG: === POLICY RESULTS ===")
	log.Printf("DEBUG: Deny policies (%d)", len(policyResult.Deny))
	for i, p := range policyResult.Deny {
		log.Printf("DEBUG:   [%d] %s/%s", i, p.Namespace, p.Name)
		for _, ref := range p.Spec.TargetRefs {
			log.Printf("DEBUG:     TargetRef: Kind=%s, Name=%s", ref.Kind, ref.Name)
		}
	}

	log.Printf("DEBUG: Allow policies (%d)", len(policyResult.Allow))
	for i, p := range policyResult.Allow {
		log.Printf("DEBUG:   [%d] %s/%s", i, p.Namespace, p.Name)
		for _, ref := range p.Spec.TargetRefs {
			log.Printf("DEBUG:     TargetRef: Kind=%s, Name=%s", ref.Kind, ref.Name)
		}
	}

	log.Printf("DEBUG: Audit policies (%d)", len(policyResult.Audit))
	log.Printf("DEBUG: Custom policies (%d)", len(policyResult.Custom))

	// Try to get more context from the logs
	log.Printf("DEBUG: === Context from Logs ===")
	log.Printf("DEBUG: Looking for relevant information in the gateway context")
	log.Printf("DEBUG: Gateway: %s/%s", gatewayNamespace, gatewayName)
	log.Printf("DEBUG: Service: %s/%s", svc.GetNamespace(), svc.GetName())
	log.Printf("DEBUG: IsWaypoint: true")

	// Log the final result
	log.Printf("Policy result: Deny=%d, Allow=%d, Audit=%d, Custom=%d",
		len(policyResult.Deny), len(policyResult.Allow),
		len(policyResult.Audit), len(policyResult.Custom))

	if len(policyResult.Deny) == 0 && len(policyResult.Allow) == 0 &&
		len(policyResult.Audit) == 0 && len(policyResult.Custom) == 0 {
		log.Printf("No applicable policies found")
		return nil
	}
	trustBundle := trustdomain.NewBundle("cluster.local", nil)
	builder := builder.New(trustBundle, nil, policyResult, builder.Option{
		IsCustomBuilder: false,
		UseFilterState:  true,
	})

	if builder == nil {
		log.Printf("Builder is nil after processing policies.")
	} else {
		log.Printf("Successfully created Authorization Builder with %d allow policies and %d deny policies",
			len(policyResult.Allow), len(policyResult.Deny))
	}

	return builder
}

func CustomNetworkFilters(
	extraFilters []*listenerv3.Filter,
	stage filters.FilterStage_Stage,
	predicate filters.FilterStage_Predicate,
) []*ir.CustomEnvoyFilter {
	customFilters := make([]*ir.CustomEnvoyFilter, 0, len(extraFilters))
	for _, f := range extraFilters {
		customFilters = append(customFilters, CustomNetworkFilter(f, stage, predicate))
	}
	return customFilters
}

func CustomNetworkFilter(
	f *listenerv3.Filter,
	stage filters.FilterStage_Stage,
	predicate filters.FilterStage_Predicate,
) *ir.CustomEnvoyFilter {
	config := f.GetTypedConfig()
	if config == nil {
		log.Printf("CustomNetworkFilter: Skipping filter %s as it has nil TypedConfig", f.Name)
		return nil
	}

	log.Printf("Attaching CustomNetworkFilter: name=%s, stage=%v, weight=%d", f.Name, stage, predicate)
	return customFiltersHelper(stage, predicate, f.Name, config)
}

func CustomHTTPFilters(
	extraFilters []*hcmv3.HttpFilter,
	stage filters.FilterStage_Stage,
	predicate filters.FilterStage_Predicate,
) []*ir.CustomEnvoyFilter {
	customFilters := make([]*ir.CustomEnvoyFilter, 0, len(extraFilters))
	for _, f := range extraFilters {
		customFilters = append(customFilters, CustomHTTPFilter(f, stage, predicate))
	}
	return customFilters
}

func CustomHTTPFilter(
	f *hcmv3.HttpFilter,
	stage filters.FilterStage_Stage,
	predicate filters.FilterStage_Predicate,
) *ir.CustomEnvoyFilter {
	config := f.GetTypedConfig()
	if config == nil {
		log.Printf("CustomHTTPFilter: Skipping filter %s as it has nil TypedConfig", f.Name)
		return nil
	}

	log.Printf("Attaching CustomHTTPFilter: name=%s, stage=%v, weight=%d", f.Name, stage, predicate)
	return customFiltersHelper(stage, predicate, f.Name, config)
}

func customFiltersHelper(
	stage filters.FilterStage_Stage,
	predicate filters.FilterStage_Predicate,
	name string,
	config *anypb.Any,
) *ir.CustomEnvoyFilter {
	return &ir.CustomEnvoyFilter{
		FilterStage: plugins.FilterStage[plugins.WellKnownFilterStage]{
			RelativeTo: plugins.WellKnownFilterStage(int(stage)),
			Weight:     int(predicate),
		},
		Name:   name,
		Config: config,
	}
}
