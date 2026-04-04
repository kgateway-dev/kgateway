package trafficpolicy

import (
	"fmt"

	cncfmatcherv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	envoyrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	envoyauthz "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	sharedv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// rbacIr is the internal representation of an RBAC policy.
type rbacIR struct {
	rbacConfig *envoyauthz.RBACPerRoute
}

func (r *rbacIR) Equals(other *rbacIR) bool {
	if r == nil && other == nil {
		return true
	}
	if r == nil || other == nil {
		return false
	}
	return proto.Equal(r.rbacConfig, other.rbacConfig)
}

// Validate performs validation on the rbac component.
func (r *rbacIR) Validate() error {
	if r == nil {
		return nil
	}
	if r.rbacConfig == nil {
		return nil
	}

	return r.rbacConfig.Validate()
}

// handleRBAC configures the RBAC filter and per-route RBAC configuration for a specific route
func (p *trafficPolicyPluginGwPass) handleRBAC(fcn string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, rbacIr *rbacIR) {
	if rbacIr == nil || rbacIr.rbacConfig == nil {
		return
	}

	// Add rbac filter to the chain if configured
	if p.rbacInChain == nil {
		p.rbacInChain = make(map[string]*envoyauthz.RBAC)
	}
	if _, ok := p.rbacInChain[fcn]; !ok {
		p.rbacInChain[fcn] = &envoyauthz.RBAC{}
	}

	// Add the per-route RBAC configuration to the typed filter config
	pCtxTypedFilterConfig.AddTypedConfig(rbacFilterNamePrefix, rbacIr.rbacConfig)
}

// constructRBAC translates the RBAC spec into an envoy RBAC policy and stores it in the traffic policy IR
func constructRBAC(policy *kgateway.TrafficPolicy, out *trafficPolicySpecIr) error {
	spec := policy.Spec
	if spec.RBAC == nil {
		return nil
	}

	rbacConfig, err := translateRBAC(spec.RBAC)
	if err != nil {
		return err
	}

	out.rbac = &rbacIR{
		rbacConfig: rbacConfig,
	}
	return nil
}

func translateRBAC(rbac *sharedv1alpha1.Authorization) (*envoyauthz.RBACPerRoute, error) {
	var errs []error

	// Create matcher-based RBAC configuration
	var matchers []*cncfmatcherv3.Matcher_MatcherList_FieldMatcher

	if len(rbac.Policy.MatchExpressions) > 0 {
		matcher, err := plugins.CreateCELMatcher(rbac.Policy.MatchExpressions, rbac.Action)
		if err != nil {
			errs = append(errs, err)
		}
		matchers = append(matchers, matcher)
	}

	if len(matchers) == 0 {
		// If no CEL matchers, create a simple deny-all RBAC
		return &envoyauthz.RBACPerRoute{
			Rbac: &envoyauthz.RBAC{
				Rules: &envoyrbacv3.RBAC{
					Action:   envoyrbacv3.RBAC_DENY,
					Policies: map[string]*envoyrbacv3.Policy{},
				},
			},
		}, nil
	}

	celMatcher := &cncfmatcherv3.Matcher{
		MatcherType: &cncfmatcherv3.Matcher_MatcherList_{
			MatcherList: &cncfmatcherv3.Matcher_MatcherList{
				Matchers: matchers,
			},
		},
		OnNoMatch: plugins.CreateDefaultAction(envoyrbacv3.RBAC_DENY),
	}

	res := &envoyauthz.RBACPerRoute{
		Rbac: &envoyauthz.RBAC{
			Matcher: celMatcher, // Use the Matcher field directly
		},
	}

	if len(errs) > 0 {
		return res, fmt.Errorf("RBAC policy encountered CEL matcher errors: %v", errs)
	}
	return res, nil
}
