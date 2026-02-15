package listenerpolicy

import (
	"fmt"

	"cel.dev/expr"
	cncfcorev3 "github.com/cncf/xds/go/xds/core/v3"
	cncfmatcherv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	cncftypev3 "github.com/cncf/xds/go/xds/type/v3"
	envoyrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	envoyrbacnetwork "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/rbac/v3"
	"github.com/google/cel-go/cel"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	sharedv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
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
		matcher, err := createNetworkCELMatcher(rbac.Policy.MatchExpressions, rbac.Action)
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
		OnNoMatch: createDefaultAction(getInverseRBACAction(rbac.Action)),
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

// createNetworkCELMatcher creates CEL matcher for network-level attributes
func createNetworkCELMatcher(celExprs []sharedv1alpha1.CELExpression, action sharedv1alpha1.AuthorizationPolicyAction) (*cncfmatcherv3.Matcher_MatcherList_FieldMatcher, error) {
	if len(celExprs) == 0 {
		return nil, fmt.Errorf("no CEL expressions provided")
	}

	// Create CEL match input for connection attributes
	// Note: For network-level RBAC, we use HttpAttributesCelMatchInput (despite the name)
	// as it's the generic CEL input type. Connection attributes are available in the CEL context.
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
		// Multiple expressions - create OR predicate
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

		// Create an OR predicate
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
	if action == sharedv1alpha1.AuthorizationPolicyActionDeny {
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

// parseCELExpression converts CEL string to parsed expression for Envoy
func parseCELExpression(env *cel.Env, celExpr sharedv1alpha1.CELExpression) (*expr.ParsedExpr, error) {
	if env == nil {
		return nil, fmt.Errorf("CEL environment is nil")
	}

	ast, iss := env.Parse(string(celExpr))
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

// getInverseRBACAction returns the inverse RBAC action
// This is used for OnNoMatch to ensure correct policy semantics:
// - Allow policy: match → ALLOW, no-match → DENY
// - Deny policy: match → DENY, no-match → ALLOW
func getInverseRBACAction(action sharedv1alpha1.AuthorizationPolicyAction) envoyrbacv3.RBAC_Action {
	if action == sharedv1alpha1.AuthorizationPolicyActionDeny {
		return envoyrbacv3.RBAC_ALLOW
	}
	return envoyrbacv3.RBAC_DENY
}
