package policy

import (
	"strings"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	omitcanaryhostsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/retry/host/omit_canary_hosts/v3"
	previoushostsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/retry/host/previous_hosts/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

// Registered Envoy retry host predicate extension names.
// See https://github.com/envoyproxy/envoy/tree/main/source/extensions/retry/host.
const (
	previousHostsExtensionName   = "envoy.retry_host_predicates.previous_hosts"
	omitCanaryHostsExtensionName = "envoy.retry_host_predicates.omit_canary_hosts"
)

func BuildRetryPolicy(in *kgateway.Retry) *envoyroutev3.RetryPolicy {
	if in == nil {
		return nil
	}
	policy := &envoyroutev3.RetryPolicy{
		RetryOn:              retryOnToString(in.RetryOn, len(in.StatusCodes) > 0),
		NumRetries:           wrapperspb.UInt32(uint32(in.Attempts)), //nolint:gosec // G115: retry attempts are small positive integers
		RetriableStatusCodes: retryCodesToUint32(in.StatusCodes),
	}
	if in.PerTryTimeout != nil {
		policy.PerTryTimeout = durationpb.New(in.PerTryTimeout.Duration)
	}

	if in.BackoffBaseInterval != nil {
		policy.RetryBackOff = &envoyroutev3.RetryPolicy_RetryBackOff{
			BaseInterval: durationpb.New(in.BackoffBaseInterval.Duration),
		}
	}

	policy.RetryHostPredicate = buildRetryHostPredicates(in.RetryHostPredicates)

	return policy
}

// buildRetryHostPredicates converts the given RetryHostPredicate values into their corresponding
// Envoy retry host predicate extension configs. Both supported predicates have an empty config
// proto, so the TypedConfig is always a wrapped empty message; the predicate's behavior is
// selected entirely by Name.
func buildRetryHostPredicates(in []kgateway.RetryHostPredicate) []*envoyroutev3.RetryPolicy_RetryHostPredicate {
	if len(in) == 0 {
		return nil
	}
	out := make([]*envoyroutev3.RetryPolicy_RetryHostPredicate, 0, len(in))
	for _, p := range in {
		switch p {
		case kgateway.RetryHostPredicatePreviousHosts:
			out = append(out, &envoyroutev3.RetryPolicy_RetryHostPredicate{
				Name: previousHostsExtensionName,
				ConfigType: &envoyroutev3.RetryPolicy_RetryHostPredicate_TypedConfig{
					TypedConfig: utils.MustMessageToAny(&previoushostsv3.PreviousHostsPredicate{}),
				},
			})
		case kgateway.RetryHostPredicateOmitCanaryHosts:
			out = append(out, &envoyroutev3.RetryPolicy_RetryHostPredicate{
				Name: omitCanaryHostsExtensionName,
				ConfigType: &envoyroutev3.RetryPolicy_RetryHostPredicate_TypedConfig{
					TypedConfig: utils.MustMessageToAny(&omitcanaryhostsv3.OmitCanaryHostsPredicate{}),
				},
			})
		}
	}
	return out
}

// retryOnToString converts a slice of RetryOnCondition to a comma-separated string
func retryOnToString(retryOn []kgateway.RetryOnCondition, forStatusCodes bool) string {
	retryOnSet := sets.NewString()
	for _, r := range retryOn {
		retryOnSet.Insert(string(r))
	}
	// If specific status codes are specified, implicitly configure retries on status codes
	if forStatusCodes {
		retryOnSet.Insert("retriable-status-codes")
	}
	return strings.Join(retryOnSet.List(), ",")
}

func retryCodesToUint32(codes []gwv1.HTTPRouteRetryStatusCode) []uint32 {
	if len(codes) == 0 {
		return nil
	}
	uint32Codes := make([]uint32, len(codes))
	for i, code := range codes {
		uint32Codes[i] = uint32(code) //nolint:gosec // G115: HTTP status codes are always positive integers (100-599)
	}
	return uint32Codes
}
