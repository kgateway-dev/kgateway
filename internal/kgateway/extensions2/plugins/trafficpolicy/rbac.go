package trafficpolicy

import (
	"fmt"

	"cel.dev/expr"
	cncfcorev3 "github.com/cncf/xds/go/xds/core/v3"
	cncfmatcherv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	cncftypev3 "github.com/cncf/xds/go/xds/type/v3"
	envoyrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	envoyauthz "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	"github.com/google/cel-go/cel"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
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
func constructRBAC(policy *v1alpha1.TrafficPolicy, out *trafficPolicySpecIr) error {
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

func translateRBAC(rbac *v1alpha1.RBAC) (*envoyauthz.RBACPerRoute, error) {
	var errs []error

	// Create matcher-based RBAC configuration
	var matchers []*cncfmatcherv3.Matcher_MatcherList_FieldMatcher

	if len(rbac.Policy.MatchExpressions) > 0 {
		matcher, err := createCELMatcher(rbac.Policy.MatchExpressions, rbac.Action)
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
		OnNoMatch: createDefaultAction(envoyrbacv3.RBAC_DENY),
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

func createCELMatcher(celExprs []string, action v1alpha1.AuthorizationPolicyAction) (*cncfmatcherv3.Matcher_MatcherList_FieldMatcher, error) {
	if len(celExprs) == 0 {
		return nil, fmt.Errorf("no CEL expressions provided")
	}

	// Create CEL match input
	celMatchInput, err := utils.MessageToAny(&cncfmatcherv3.HttpAttributesCelMatchInput{})
	if err != nil {
		return nil, err
	}

	celMatchInputConfig := &cncfcorev3.TypedExtensionConfig{
		Name:        "envoy.matching.inputs.cel_data_input",
		TypedConfig: celMatchInput,
	}

	// Create parsed expression
	env, err := cel.NewEnv()
	if err != nil {
		logger.Error("failed to create CEL environment", "err", err.Error())
		return nil, err
	}

	var predicate *cncfmatcherv3.Matcher_MatcherList_Predicate
	if len(celExprs) == 1 {
		// Single expression - use SinglePredicate
		celDevParsed, err := parseCELExpression(env, celExprs[0])
		if err != nil {
			return nil, fmt.Errorf("failed to parse CEL expression: %w", err)
		}

		matcher := &cncfmatcherv3.CelMatcher{
			ExprMatch: &cncftypev3.CelExpression{
				CelExprParsed: celDevParsed,
			},
		}
		pb, err := utils.MessageToAny(matcher)
		if err != nil {
			return nil, err
		}

		typedCelMatchConfig := &cncfcorev3.TypedExtensionConfig{
			Name:        "envoy.matching.matchers.cel_matcher",
			TypedConfig: pb,
		}
		predicate = &cncfmatcherv3.Matcher_MatcherList_Predicate{
			MatchType: &cncfmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_{
				SinglePredicate: &cncfmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate{
					Input: celMatchInputConfig,
					Matcher: &cncfmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_CustomMatch{
						CustomMatch: typedCelMatchConfig,
					},
				},
			},
		}
	} else {
		// Multiple expressions - create a list of predicates
		var predicates []*cncfmatcherv3.Matcher_MatcherList_Predicate

		for _, celExpr := range celExprs {
			celDevParsed, err := parseCELExpression(env, celExpr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse CEL expression: %w", err)
			}

			matcher := &cncfmatcherv3.CelMatcher{
				ExprMatch: &cncftypev3.CelExpression{
					CelExprParsed: celDevParsed,
				},
			}
			pb, err := utils.MessageToAny(matcher)
			if err != nil {
				return nil, err
			}

			typedCelMatchConfig := &cncfcorev3.TypedExtensionConfig{
				Name:        "envoy.matching.matchers.cel_matcher",
				TypedConfig: pb,
			}

			singlePredicate := &cncfmatcherv3.Matcher_MatcherList_Predicate{
				MatchType: &cncfmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_{
					SinglePredicate: &cncfmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate{
						Input: celMatchInputConfig,
						Matcher: &cncfmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_CustomMatch{
							CustomMatch: typedCelMatchConfig,
						},
					},
				},
			}
			predicates = append(predicates, singlePredicate)
		}

		// Create an OR predicate that contains all the single predicates
		predicate = &cncfmatcherv3.Matcher_MatcherList_Predicate{
			MatchType: &cncfmatcherv3.Matcher_MatcherList_Predicate_OrMatcher{
				OrMatcher: &cncfmatcherv3.Matcher_MatcherList_Predicate_PredicateList{
					Predicate: predicates,
				},
			},
		}
	}

	// Determine the action based on policy action
	var onMatchAction *cncfmatcherv3.Matcher_OnMatch
	if action == v1alpha1.AuthorizationPolicyActionDeny {
		onMatchAction = createMatchAction(envoyrbacv3.RBAC_DENY)
	} else {
		onMatchAction = createMatchAction(envoyrbacv3.RBAC_ALLOW)
	}

	return &cncfmatcherv3.Matcher_MatcherList_FieldMatcher{
		Predicate: predicate,
		OnMatch:   onMatchAction,
	}, nil
}

func createMatchAction(action envoyrbacv3.RBAC_Action) *cncfmatcherv3.Matcher_OnMatch {
	actionName := "allow-request"
	if action == envoyrbacv3.RBAC_DENY {
		actionName = "deny-request"
	}

	rbacAction := &envoyrbacv3.Action{
		Name:   actionName,
		Action: action,
	}

	actionAny, _ := utils.MessageToAny(rbacAction)

	return &cncfmatcherv3.Matcher_OnMatch{
		OnMatch: &cncfmatcherv3.Matcher_OnMatch_Action{
			Action: &cncfcorev3.TypedExtensionConfig{
				Name:        "envoy.filters.rbac.action",
				TypedConfig: actionAny,
			},
		},
	}
}

func createDefaultAction(action envoyrbacv3.RBAC_Action) *cncfmatcherv3.Matcher_OnMatch {
	actionName := "allow-request"
	if action == envoyrbacv3.RBAC_DENY {
		actionName = "deny-request"
	}

	rbacAction := &envoyrbacv3.Action{
		Name:   actionName,
		Action: action,
	}

	actionAny, _ := utils.MessageToAny(rbacAction)

	return &cncfmatcherv3.Matcher_OnMatch{
		OnMatch: &cncfmatcherv3.Matcher_OnMatch_Action{
			Action: &cncfcorev3.TypedExtensionConfig{
				Name:        "action",
				TypedConfig: actionAny,
			},
		},
	}
}

// parseCELExpression takes a CEL expression string and converts it to a parsed expression
// for use in Envoy matchers. It handles the conversion between different protobuf types.
func parseCELExpression(env *cel.Env, celExpr string) (*expr.ParsedExpr, error) {
	if env == nil {
		return nil, fmt.Errorf("CEL environment is nil")
	}

	ast, iss := env.Parse(celExpr)
	if iss.Err() != nil {
		logger.Error("parse error", "err", iss.Err())
		return nil, iss.Err()
	}

	parsedExpr, err := cel.AstToParsedExpr(ast)
	if err != nil {
		logger.Error("failed to convert AST to parsed expression", "err", err.Error())
		return nil, err
	}

	// Marshal from google.golang.org/genproto
	data, err := proto.Marshal(parsedExpr)
	if err != nil {
		logger.Error("marshal err", "err", err.Error())
		return nil, err
	}

	// Unmarshal into cel.dev/expr/v1alpha1
	var celDevParsed expr.ParsedExpr
	if err := proto.Unmarshal(data, &celDevParsed); err != nil {
		logger.Error("unmarshal err", "err", err.Error())
		return nil, err
	}

	return &celDevParsed, nil
}
