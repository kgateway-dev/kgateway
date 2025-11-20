package utils

import (
	"fmt"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoymatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ToEnvoyHeaderMatcher converts a Gateway API HTTPHeaderMatch to an Envoy HeaderMatcher
func ToEnvoyHeaderMatcher(header gwv1.HTTPHeaderMatch) (*envoyroutev3.HeaderMatcher, error) {
	switch *header.Type {
	case gwv1.HeaderMatchExact:
		return &envoyroutev3.HeaderMatcher{
			Name: string(header.Name),
			HeaderMatchSpecifier: &envoyroutev3.HeaderMatcher_StringMatch{
				StringMatch: &envoymatcherv3.StringMatcher{
					IgnoreCase: false,
					MatchPattern: &envoymatcherv3.StringMatcher_Exact{
						Exact: header.Value,
					},
				},
			},
		}, nil
	case gwv1.HeaderMatchRegularExpression:
		return &envoyroutev3.HeaderMatcher{
			Name: string(header.Name),
			HeaderMatchSpecifier: &envoyroutev3.HeaderMatcher_StringMatch{
				StringMatch: &envoymatcherv3.StringMatcher{
					IgnoreCase: false,
					MatchPattern: &envoymatcherv3.StringMatcher_SafeRegex{
						SafeRegex: &envoymatcherv3.RegexMatcher{
							Regex: header.Value,
						},
					},
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported header match type: %v", *header.Type)
	}
}

// ToEnvoyHeaderMatchers converts a list of Gateway API HTTPHeaderMatch to a list of Envoy HeaderMatcher
func ToEnvoyHeaderMatchers(headers []gwv1.HTTPHeaderMatch) ([]*envoyroutev3.HeaderMatcher, error) {
	matchers := make([]*envoyroutev3.HeaderMatcher, 0, len(headers))
	for _, header := range headers {
		matcher, err := ToEnvoyHeaderMatcher(header)
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, matcher)
	}
	return matchers, nil
}
