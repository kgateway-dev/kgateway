package policy

import (
	"strings"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func BuildRetryPolicy(in *v1alpha1.Retry) *envoyroutev3.RetryPolicy {
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

	return policy
}

// retryOnToString converts a slice of RetryOnCondition to a comma-separated string
func retryOnToString(retryOn []v1alpha1.RetryOnCondition, forStatusCodes bool) string {
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
