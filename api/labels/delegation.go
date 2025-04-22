package labels

import "github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"

// DelegationLabelSelector is the label used to select HTTPRoutes to delegate to
// using a single label key-value pair
const DelegationLabelSelector = "delegation.kgateway.dev/label"

// DelegationLabelSelectorWildcardNamespace wildcards the namespace to select delegatee routes
// using the DelegationLabelSelector label.
// Note: this must be a valid RFC 1123 DNS label
var DelegationLabelSelectorWildcardNamespace = envutils.GetOrDefault("DELEGATION_WILDCARD_NAMESPACE", "all", false)
