package listenerpolicy

import (
	"fmt"

	cncfmatcherv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	envoyrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	envoyrbacnetwork "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/rbac/v3"
	"google.golang.org/protobuf/types/known/anypb"

	sharedv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// translateNetworkRbac converts shared.Authorization to Envoy network RBAC filter config
func translateNetworkRbac(rbac *sharedv1alpha1.Authorization, objSrc ir.ObjectSource) (*anypb.Any, error) {
	if rbac == nil {
		return nil, nil
	}

	var errs []error

	// Create matcher-based RBAC configuration (same as HTTP RBAC)
	var matchers []*cncfmatcherv3.Matcher_MatcherList_FieldMatcher

	if len(rbac.Policy.MatchExpressions) > 0 {
		matcher, err := plugins.CreateCELMatcher(rbac.Policy.MatchExpressions, rbac.Action)
		if err != nil {
			errs = append(errs, err)
		}
		if matcher != nil {
			matchers = append(matchers, matcher)
		}
	}

	if len(matchers) == 0 {
		// If there were errors during matcher creation, return them
		if len(errs) > 0 {
			return nil, fmt.Errorf("network RBAC policy encountered CEL matcher errors: %v", errs)
		}
		// If no CEL matchers, create a simple deny-all RBAC
		rbacConfig := &envoyrbacnetwork.RBAC{
			StatPrefix: fmt.Sprintf("%s_%s_network_rbac", objSrc.Namespace, objSrc.Name),
			Rules: &envoyrbacv3.RBAC{
				Action:   envoyrbacv3.RBAC_DENY,
				Policies: map[string]*envoyrbacv3.Policy{},
			},
		}
		anyConfig, err := utils.MessageToAny(rbacConfig)
		if err != nil {
			return nil, err
		}
		return anyConfig, nil
	}

	celMatcher := &cncfmatcherv3.Matcher{
		MatcherType: &cncfmatcherv3.Matcher_MatcherList_{
			MatcherList: &cncfmatcherv3.Matcher_MatcherList{
				Matchers: matchers,
			},
		},
		OnNoMatch: plugins.CreateDefaultAction(getInverseRBACAction(rbac.Action)),
	}

	rbacConfig := &envoyrbacnetwork.RBAC{
		StatPrefix: fmt.Sprintf("%s_%s_network_rbac", objSrc.Namespace, objSrc.Name),
		Matcher:    celMatcher,
	}

	if len(errs) > 0 {
		anyConfig, err := utils.MessageToAny(rbacConfig)
		if err != nil {
			return nil, err
		}
		return anyConfig, fmt.Errorf("network RBAC policy encountered CEL matcher errors: %v", errs)
	}

	return utils.MessageToAny(rbacConfig)
}

// getInverseRBACAction returns the inverse RBAC action
// This is used for OnNoMatch to ensure correct policy semantics
func getInverseRBACAction(action sharedv1alpha1.AuthorizationPolicyAction) envoyrbacv3.RBAC_Action {
	if action == sharedv1alpha1.AuthorizationPolicyActionDeny {
		return envoyrbacv3.RBAC_ALLOW
	}
	return envoyrbacv3.RBAC_DENY
}
