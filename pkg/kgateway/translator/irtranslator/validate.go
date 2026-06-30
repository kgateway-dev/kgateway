package irtranslator

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/regexutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
	"github.com/kgateway-dev/kgateway/v2/pkg/xds/bootstrap"
)

var (
	// invalidPathSequences are path sequences that should not be contained in a path
	invalidPathSequences = []string{"//", "/./", "/../", "%2f", "%2F", "#"}
	// invalidPathSuffixes are path suffixes that should not be at the end of a path
	invalidPathSuffixes = []string{"/..", "/."}
	// validCharacterRegex pattern based off RFC 3986 similar to kubernetes-sigs/gateway-api implementation
	// for finding "pchar" characters = unreserved / pct-encoded / sub-delims / ":" / "@"
	validPathRegexCharacters = "^(?:([A-Za-z0-9/:@._~!$&'()*+,:=;-]*|[%][0-9a-fA-F]{2}))*$"

	validPathRegex = regexp.MustCompile(validPathRegexCharacters)

	NoDestinationSpecifiedError       = errors.New("must specify at least one weighted destination for multi destination routes")
	ValidRoutePatternError            = fmt.Errorf("must only contain valid characters matching pattern %s", validPathRegexCharacters)
	PathContainsInvalidCharacterError = func(s, invalid string) error {
		return fmt.Errorf("path [%s] cannot contain [%s]", s, invalid)
	}
	PathEndsWithInvalidCharactersError = func(s, invalid string) error {
		return fmt.Errorf("path [%s] cannot end with [%s]", s, invalid)
	}

	// ErrInvalidMatcher is returned when the matcher is invalid.
	ErrInvalidMatcher = errors.New("invalid matcher configuration")
	// ErrInvalidRoute is returned when the route is invalid.
	ErrInvalidRoute = errors.New("invalid route configuration")
)

// validateRoute performs RDS validation against a single route that's been translated from the IR.
// It provides validation checks that prevent invalid routes from being sent to the xDS server.
//
// The validation pipeline runs lightweight checks regardless of route replacement mode.
// These include basic Envoy route property validation such as paths, prefixes, and weighted clusters,
// along with quick checks for common issues that could cause problems.
//
// In strict mode, generated Gateway API matchers are checked in-process where possible. The full route
// is then validated against Envoy in a single invocation. If that invocation fails, matcher validation
// disambiguates whether the failure originates in the route's match block (drop the route) or in its
// action (replace with a direct response). Matcher shapes this code cannot prove safe fall back to Envoy
// matcher-only validation.
//
// This two-tiered approach is necessary because Envoy validate mode output does not provide
// enough information to determine if the error is due to an invalid matcher (drop route)
// or other route configuration issues (replace with direct response).
func validateRoute(
	ctx context.Context,
	route *envoyroutev3.Route,
	v validator.Validator,
	mode apisettings.ValidationMode,
) error {
	if err := validateRoutePreEnvoy(route, mode); err != nil {
		return err
	}
	if mode != apisettings.ValidationStrict {
		return nil
	}
	return validateRouteWithEnvoy(ctx, route, v)
}

func validateRoutePreEnvoy(route *envoyroutev3.Route, mode apisettings.ValidationMode) error {
	if route == nil {
		return fmt.Errorf("route cannot be nil for RDS validation")
	}
	if err := validateEnvoyRoute(route); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRoute, err)
	}
	if mode != apisettings.ValidationStrict {
		return nil
	}
	if handled, err := validateGeneratedMatcher(route.GetMatch()); handled && err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidMatcher, err)
	}
	return nil
}

func validateRouteWithEnvoy(
	ctx context.Context,
	route *envoyroutev3.Route,
	v validator.Validator,
) error {
	if route == nil {
		return fmt.Errorf("route cannot be nil for RDS validation")
	}
	fullErr := validateFullRoutes(ctx, []*envoyroutev3.Route{route}, v)
	if fullErr == nil {
		return nil
	}
	// Only run the matcher-only validation when the full route already failed,
	// purely to attribute the failure to ErrInvalidMatcher vs ErrInvalidRoute.
	if matcherErr := validateMatcherOnlyEnvoy(ctx, route, v); matcherErr != nil {
		return fmt.Errorf("%w: %w", ErrInvalidMatcher, matcherErr)
	}
	return fmt.Errorf("%w: %w", ErrInvalidRoute, fullErr)
}

// validateEnvoyRoute performs basic validation on Envoy route properties
func validateEnvoyRoute(r *envoyroutev3.Route) error {
	var errs []error
	match := r.GetMatch()
	route := r.GetRoute()
	re := r.GetRedirect()
	validatePath(match.GetPath(), &errs)
	validatePath(match.GetPrefix(), &errs)
	validatePath(match.GetPathSeparatedPrefix(), &errs)
	validatePath(re.GetPathRedirect(), &errs)
	validatePath(re.GetHostRedirect(), &errs)
	validatePath(re.GetSchemeRedirect(), &errs)
	validatePrefixRewrite(route.GetPrefixRewrite(), &errs)
	validatePrefixRewrite(re.GetPrefixRewrite(), &errs)
	validateWeightedClusters(route.GetWeightedClusters().GetClusters(), &errs)
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("error %s: %w", r.GetName(), errors.Join(errs...))
}

// validateWeightedClusters validates that at least one cluster has a non-zero weight
func validateWeightedClusters(clusters []*envoyroutev3.WeightedCluster_ClusterWeight, errs *[]error) {
	if len(clusters) == 0 {
		return
	}

	allZeroWeight := true
	for _, cluster := range clusters {
		if cluster.GetWeight().GetValue() > 0 {
			allZeroWeight = false
			break
		}
	}
	if allZeroWeight {
		*errs = append(*errs, errors.New("All backend weights are 0. At least one backendRef in the HTTPRoute rule must specify a non-zero weight"))
	}
}

func validatePath(path string, errs *[]error) {
	if err := ValidateRoutePath(path); err != nil {
		*errs = append(*errs, fmt.Errorf("the \"%s\" path is invalid: %w", path, err))
	}
}

func validatePrefixRewrite(rewrite string, errs *[]error) {
	if err := ValidatePrefixRewrite(rewrite); err != nil {
		*errs = append(*errs, fmt.Errorf("the rewrite %s is invalid: %w", rewrite, err))
	}
}

// ValidatePrefixRewrite will validate the rewrite using url.Parse. Then it will evaluate the Path of the rewrite.
func ValidatePrefixRewrite(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	return ValidateRoutePath(u.Path)
}

// ValidateRoutePath will validate a string for all characters according to RFC 3986
// "pchar" characters = unreserved / pct-encoded / sub-delims / ":" / "@"
// https://www.rfc-editor.org/rfc/rfc3986/
func ValidateRoutePath(s string) error {
	if s == "" {
		return nil
	}
	if !validPathRegex.Match([]byte(s)) {
		return ValidRoutePatternError
	}
	for _, invalid := range invalidPathSequences {
		if strings.Contains(s, invalid) {
			return PathContainsInvalidCharacterError(s, invalid)
		}
	}
	for _, invalid := range invalidPathSuffixes {
		if strings.HasSuffix(s, invalid) {
			return PathEndsWithInvalidCharactersError(s, invalid)
		}
	}
	return nil
}

// runValidation executes the common validation sequence: build bootstrap, marshal to JSON, and validate.
// It takes a configured bootstrap builder and returns an error if any step fails.
func runValidation(
	ctx context.Context,
	v validator.Validator,
	builder *bootstrap.ConfigBuilder,
	caller validator.ValidationCaller,
) error {
	bootstrap, err := builder.Build()
	if err != nil {
		return fmt.Errorf("failed to build bootstrap config: %w", err)
	}
	if err := v.Validate(validator.WithValidationCaller(ctx, caller), bootstrap); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return nil
}

func validateMatcherOnlyEnvoy(ctx context.Context, route *envoyroutev3.Route, v validator.Validator) error {
	clusterName := "dummy-cluster"
	builder := bootstrap.New()
	builder.AddRoute(&envoyroutev3.Route{
		Name:  route.GetName(),
		Match: route.GetMatch(),
		Action: &envoyroutev3.Route_Route{
			Route: &envoyroutev3.RouteAction{
				ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{
					Cluster: clusterName,
				},
			},
		},
	})
	builder.AddCluster(&envoyclusterv3.Cluster{
		Name: clusterName,
	})
	return runValidation(ctx, v, builder, validator.CallerRouteMatcher)
}

// validateFullRoutes validates a set of complete route configurations in one Envoy invocation.
func validateFullRoutes(ctx context.Context, routes []*envoyroutev3.Route, v validator.Validator) error {
	builder := bootstrap.New()
	clusterNames := make([]string, 0)
	// A batched route bootstrap can reference the same backend cluster from many routes.
	// Track names here so validation adds only one stub Cluster per unique Envoy cluster name.
	seenClusterNames := make(map[string]struct{})
	for _, route := range routes {
		builder.AddRoute(route)
		clusterNames = appendClusterNames(clusterNames, seenClusterNames, route)
	}
	stubClusters := createStubClusters(clusterNames)
	for _, cluster := range stubClusters {
		builder.AddCluster(cluster)
	}

	return runValidation(ctx, v, builder, validator.CallerRouteFull)
}

func validateGeneratedMatcher(match *envoyroutev3.RouteMatch) (bool, error) {
	if match == nil {
		return false, nil
	}
	if match.GetRuntimeFraction() != nil || match.GetGrpc() != nil || match.GetTlsContext() != nil || match.GetDynamicMetadata() != nil {
		return false, nil
	}

	if handled, err := validateGeneratedPathMatcher(match); !handled || err != nil {
		return handled, err
	}
	for _, header := range match.GetHeaders() {
		if handled, err := validateGeneratedHeaderMatcher(header); !handled || err != nil {
			return handled, err
		}
	}
	for _, query := range match.GetQueryParameters() {
		if handled, err := validateGeneratedQueryMatcher(query); !handled || err != nil {
			return handled, err
		}
	}

	return true, nil
}

func validateGeneratedPathMatcher(match *envoyroutev3.RouteMatch) (bool, error) {
	switch path := match.GetPathSpecifier().(type) {
	case *envoyroutev3.RouteMatch_Path:
		return true, nil
	case *envoyroutev3.RouteMatch_Prefix:
		return true, nil
	case *envoyroutev3.RouteMatch_PathSeparatedPrefix:
		if !isValidPathSeparated(path.PathSeparatedPrefix) {
			return true, fmt.Errorf("path separated prefix %q is invalid", path.PathSeparatedPrefix)
		}
		return true, nil
	case *envoyroutev3.RouteMatch_SafeRegex:
		return true, validateRegexMatcher(path.SafeRegex, "path regex")
	default:
		return false, nil
	}
}

func validateGeneratedHeaderMatcher(header *envoyroutev3.HeaderMatcher) (bool, error) {
	if header == nil {
		return false, nil
	}
	switch matcher := header.GetHeaderMatchSpecifier().(type) {
	case *envoyroutev3.HeaderMatcher_PresentMatch:
		return true, nil
	case *envoyroutev3.HeaderMatcher_StringMatch:
		return validateGeneratedStringMatcher(matcher.StringMatch, fmt.Sprintf("header %q", header.GetName()))
	default:
		return false, nil
	}
}

func validateGeneratedQueryMatcher(query *envoyroutev3.QueryParameterMatcher) (bool, error) {
	if query == nil {
		return false, nil
	}
	switch matcher := query.GetQueryParameterMatchSpecifier().(type) {
	case *envoyroutev3.QueryParameterMatcher_PresentMatch:
		return true, nil
	case *envoyroutev3.QueryParameterMatcher_StringMatch:
		return validateGeneratedStringMatcher(matcher.StringMatch, fmt.Sprintf("query parameter %q", query.GetName()))
	default:
		return false, nil
	}
}

func validateGeneratedStringMatcher(matcher *envoy_type_matcher_v3.StringMatcher, field string) (bool, error) {
	if matcher == nil {
		return false, nil
	}
	switch pattern := matcher.GetMatchPattern().(type) {
	case *envoy_type_matcher_v3.StringMatcher_Exact:
		return true, nil
	case *envoy_type_matcher_v3.StringMatcher_SafeRegex:
		return true, validateRegexMatcher(pattern.SafeRegex, field+" regex")
	default:
		return false, nil
	}
}

// validateRegexMatcher checks RE2 *syntax* only. Envoy's RE2 max_program_size
// (default 100) is the compiled-instruction count, which Go's regexp cannot
// reproduce, so an oversized-but-syntactically-valid regex passes here and is
// caught by the authoritative Envoy validation in validateFullRoutes.
func validateRegexMatcher(matcher *envoy_type_matcher_v3.RegexMatcher, field string) error {
	if matcher == nil {
		return fmt.Errorf("%s is missing", field)
	}
	if err := regexutils.CheckRegexString(matcher.GetRegex()); err != nil {
		return fmt.Errorf(
			"validation failed: %w: error initializing configuration '/dev/fd/0': %s",
			validator.ErrInvalidXDS,
			envoyRegexError(matcher.GetRegex(), err),
		)
	}
	return nil
}

func envoyRegexError(pattern string, err error) string {
	errText := err.Error()
	if strings.Contains(errText, "missing closing ]") {
		if bracket := strings.Index(pattern, "["); bracket >= 0 {
			pattern = pattern[bracket:]
		}
		return "missing ]: " + pattern
	}
	if strings.Contains(errText, "invalid named capture") {
		return "invalid named capture group: " + pattern
	}
	return errText
}

// createStubClusters creates minimal cluster definitions for validation purposes.
// These clusters have just the name field set, which is sufficient for RDS validation.
func createStubClusters(clusterNames []string) []*envoyclusterv3.Cluster {
	var clusters []*envoyclusterv3.Cluster
	for _, name := range clusterNames {
		if name != "" {
			clusters = append(clusters, &envoyclusterv3.Cluster{
				Name: name,
			})
		}
	}
	return clusters
}

func appendClusterNames(clusterNames []string, seen map[string]struct{}, route *envoyroutev3.Route) []string {
	if route == nil {
		return clusterNames
	}
	appendClusterName := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		clusterNames = append(clusterNames, name)
	}
	switch action := route.GetAction().(type) {
	case *envoyroutev3.Route_Route:
		if action.Route != nil {
			switch clusterSpec := action.Route.GetClusterSpecifier().(type) {
			case *envoyroutev3.RouteAction_Cluster:
				appendClusterName(clusterSpec.Cluster)
			case *envoyroutev3.RouteAction_WeightedClusters:
				if clusterSpec.WeightedClusters != nil {
					for _, weightedCluster := range clusterSpec.WeightedClusters.GetClusters() {
						appendClusterName(weightedCluster.GetName())
					}
				}
			}
		}
	}
	return clusterNames
}
