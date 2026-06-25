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

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
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

// validateRoute performs RDS validation against the given route that's been translated from the IR.
// It provides validation checks that prevent invalid routes from being sent to the xDS server.
//
// The validation pipeline runs lightweight checks regardless of route replacement mode.
// These include basic Envoy route property validation such as paths, prefixes, and weighted clusters,
// along with quick checks for common issues that could cause problems.
//
// In strict mode, the full route is validated against envoy in a single invocation. If that
// invocation fails, a second matcher-only invocation disambiguates whether the failure
// originates in the route's match block (drop the route) or in its action (replace with a
// direct response). Valid routes, the common case, pay only one envoy invocation; invalid
// routes pay two.
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
	if route == nil {
		return fmt.Errorf("route cannot be nil for RDS validation")
	}
	if err := validateEnvoyRoute(route); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRoute, err)
	}
	if mode != apisettings.ValidationStrict {
		return nil
	}
	fullErr := validateFullRoute(ctx, route, v)
	if fullErr == nil {
		return nil
	}
	// Only run the matcher-only validation when the full route already failed,
	// purely to attribute the failure to ErrInvalidMatcher vs ErrInvalidRoute.
	if matcherErr := validateMatcherOnly(ctx, route, v); matcherErr != nil {
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

func validateMatcherOnly(ctx context.Context, route *envoyroutev3.Route, v validator.Validator) error {
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

// validateFullRoute validates the complete route configuration.
func validateFullRoute(ctx context.Context, route *envoyroutev3.Route, v validator.Validator) error {
	builder := bootstrap.New()
	builder.AddRoute(route)
	stubClusters := createStubClusters(extractClusterNames(route))
	for _, cluster := range stubClusters {
		builder.AddCluster(cluster)
	}

	return runValidation(ctx, v, builder, validator.CallerRouteFull)
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

// extractClusterNames extracts all cluster names referenced by a route,
// handling both single cluster routes and weighted cluster routes.
// Returns a slice of unique cluster names to prevent redundant stub cluster creation.
func extractClusterNames(route *envoyroutev3.Route) []string {
	clusterNameSet := make(map[string]struct{})
	if route == nil {
		return []string{}
	}
	switch action := route.GetAction().(type) {
	case *envoyroutev3.Route_Route:
		if action.Route != nil {
			switch clusterSpec := action.Route.GetClusterSpecifier().(type) {
			case *envoyroutev3.RouteAction_Cluster:
				if clusterSpec.Cluster != "" {
					clusterNameSet[clusterSpec.Cluster] = struct{}{}
				}
			case *envoyroutev3.RouteAction_WeightedClusters:
				if clusterSpec.WeightedClusters != nil {
					for _, weightedCluster := range clusterSpec.WeightedClusters.GetClusters() {
						if weightedCluster.GetName() != "" {
							clusterNameSet[weightedCluster.GetName()] = struct{}{}
						}
					}
				}
			}
		}
	}
	clusterNames := make([]string, 0, len(clusterNameSet))
	for name := range clusterNameSet {
		clusterNames = append(clusterNames, name)
	}
	return clusterNames
}
