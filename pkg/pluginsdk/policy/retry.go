package policy

import (
	"strings"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
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

	if in.RateLimitedBackOff != nil {
		policy.RateLimitedRetryBackOff = buildRateLimitedRetryBackOff(in.RateLimitedBackOff)
	}

	return policy
}

func buildRateLimitedRetryBackOff(in *kgateway.RateLimitedRetryBackOff) *envoyroutev3.RetryPolicy_RateLimitedRetryBackOff {
	backOff := &envoyroutev3.RetryPolicy_RateLimitedRetryBackOff{
		ResetHeaders: resetHeadersToEnvoy(in.ResetHeaders),
	}
	if in.MaxInterval != nil {
		backOff.MaxInterval = durationpb.New(in.MaxInterval.Duration)
	}
	return backOff
}

func resetHeadersToEnvoy(headers []kgateway.ResetHeader) []*envoyroutev3.RetryPolicy_ResetHeader {
	if len(headers) == 0 {
		return nil
	}
	out := make([]*envoyroutev3.RetryPolicy_ResetHeader, len(headers))
	for i, h := range headers {
		out[i] = &envoyroutev3.RetryPolicy_ResetHeader{
			Name:   string(h.Name),
			Format: resetHeaderFormatToEnvoy(h.Format),
		}
	}
	return out
}

func resetHeaderFormatToEnvoy(format kgateway.ResetHeaderFormat) envoyroutev3.RetryPolicy_ResetHeaderFormat {
	switch format {
	case kgateway.ResetHeaderFormatUnixTimestamp:
		return envoyroutev3.RetryPolicy_UNIX_TIMESTAMP
	case kgateway.ResetHeaderFormatSeconds:
		return envoyroutev3.RetryPolicy_SECONDS
	default:
		// proto3 enum zero-value semantics: SECONDS is RetryPolicy_ResetHeaderFormat's
		// zero value, so this is the correct fallback for any unrecognized format
		// (CRD validation should prevent this in practice).
		return envoyroutev3.RetryPolicy_SECONDS
	}
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
