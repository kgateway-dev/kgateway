package plugins

import (
	"fmt"

	"cel.dev/expr"
	cncfcorev3 "github.com/cncf/xds/go/xds/core/v3"
	cncfmatcherv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	cncftypev3 "github.com/cncf/xds/go/xds/type/v3"
	envoyrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	"github.com/google/cel-go/cel"
	"google.golang.org/protobuf/proto"

	sharedv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

// CreateMatchAction creates OnMatch action for RBAC matchers.
func CreateMatchAction(action envoyrbacv3.RBAC_Action) *cncfmatcherv3.Matcher_OnMatch {
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

// CreateDefaultAction creates default OnMatch action for RBAC matchers.
func CreateDefaultAction(action envoyrbacv3.RBAC_Action) *cncfmatcherv3.Matcher_OnMatch {
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
func ParseCELExpression(env *cel.Env, celExpr sharedv1alpha1.CELExpression) (*expr.ParsedExpr, error) {
	if env == nil {
		return nil, fmt.Errorf("CEL environment is nil")
	}

	ast, iss := env.Parse(string(celExpr))
	if iss.Err() != nil {
		return nil, iss.Err()
	}

	parsedExpr, err := cel.AstToParsedExpr(ast)
	if err != nil {
		return nil, err
	}

	// Marshal from google.golang.org/genproto
	data, err := proto.Marshal(parsedExpr)
	if err != nil {
		return nil, err
	}

	// Unmarshal into cel.dev/expr/v1alpha1
	var celDevParsed expr.ParsedExpr
	if err := proto.Unmarshal(data, &celDevParsed); err != nil {
		return nil, err
	}

	return &celDevParsed, nil
}

// CreateCELMatcher creates a CEL matcher for RBAC policies from CEL expressions.
func CreateCELMatcher(celExprs []sharedv1alpha1.CELExpression, action sharedv1alpha1.AuthorizationPolicyAction) (*cncfmatcherv3.Matcher_MatcherList_FieldMatcher, error) {
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
		return nil, err
	}

	var predicate *cncfmatcherv3.Matcher_MatcherList_Predicate
	if len(celExprs) == 1 {
		// Single expression - use SinglePredicate
		celDevParsed, err := ParseCELExpression(env, celExprs[0])
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
			celDevParsed, err := ParseCELExpression(env, celExpr)
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
	if action == sharedv1alpha1.AuthorizationPolicyActionDeny {
		onMatchAction = CreateMatchAction(envoyrbacv3.RBAC_DENY)
	} else {
		onMatchAction = CreateMatchAction(envoyrbacv3.RBAC_ALLOW)
	}

	return &cncfmatcherv3.Matcher_MatcherList_FieldMatcher{
		Predicate: predicate,
		OnMatch:   onMatchAction,
	}, nil
}
