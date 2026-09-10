package trafficpolicy

import (
	"fmt"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

const (
	pathDescriptorKey          = "path"
	remoteAddressDescriptorKey = "remote_address"
)

// translateRateLimitDescriptorEntry returns both halves of Envoy's local descriptor model:
// the action that derives a descriptor entry from a request and the entry that matches it to
// a local token bucket. Global rate limiting only uses the action.
func translateRateLimitDescriptorEntry(entry kgateway.RateLimitDescriptorEntry) (
	*envoyroutev3.RateLimit_Action,
	*ratelimitv3.RateLimitDescriptor_Entry,
	error,
) {
	action := &envoyroutev3.RateLimit_Action{}
	localEntry := &ratelimitv3.RateLimitDescriptor_Entry{}

	switch entry.Type {
	case kgateway.RateLimitDescriptorEntryTypeGeneric:
		if entry.Generic == nil {
			return nil, nil, fmt.Errorf("generic entry requires Generic field to be set")
		}
		action.ActionSpecifier = &envoyroutev3.RateLimit_Action_GenericKey_{
			GenericKey: &envoyroutev3.RateLimit_Action_GenericKey{
				DescriptorKey:   entry.Generic.Key,
				DescriptorValue: entry.Generic.Value,
			},
		}
		localEntry.Key = entry.Generic.Key
		localEntry.Value = entry.Generic.Value
	case kgateway.RateLimitDescriptorEntryTypeHeader:
		if entry.Header == nil {
			return nil, nil, fmt.Errorf("header entry requires Header field to be set")
		}
		action.ActionSpecifier = &envoyroutev3.RateLimit_Action_RequestHeaders_{
			RequestHeaders: &envoyroutev3.RateLimit_Action_RequestHeaders{
				HeaderName:    *entry.Header,
				DescriptorKey: *entry.Header,
			},
		}
		localEntry.Key = *entry.Header
	case kgateway.RateLimitDescriptorEntryTypeRemoteAddress:
		action.ActionSpecifier = &envoyroutev3.RateLimit_Action_RemoteAddress_{
			RemoteAddress: &envoyroutev3.RateLimit_Action_RemoteAddress{},
		}
		localEntry.Key = remoteAddressDescriptorKey
	case kgateway.RateLimitDescriptorEntryTypePath:
		action.ActionSpecifier = &envoyroutev3.RateLimit_Action_RequestHeaders_{
			RequestHeaders: &envoyroutev3.RateLimit_Action_RequestHeaders{
				HeaderName:    ":path",
				DescriptorKey: pathDescriptorKey,
			},
		}
		localEntry.Key = pathDescriptorKey
	default:
		return nil, nil, fmt.Errorf("unsupported entry type: %s", entry.Type)
	}

	return action, localEntry, nil
}
