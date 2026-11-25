package backendconfigpolicy

import "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"

// Condition type and reasons for BackendConfigPolicy partial overrides.
const (
	// ConditionOverridden is set on a BackendConfigPolicy ancestor when one or
	// more of its subfields were dropped in favor of a higher priority policy.
	// The condition is only emitted when there is an actual override; absence
	// means no subfield was overridden.
	ConditionOverridden = reporter.PolicyConditionOverridden

	// ReasonConflictedWithBackendTLSPolicy is the Overridden=True reason used
	// when BackendConfigPolicy.spec.tls is dropped because a BackendTLSPolicy
	// also targets the same backend.
	ReasonConflictedWithBackendTLSPolicy = "ConflictedWithBackendTLSPolicy"

	// ReasonConflictedWithBackendConfigPolicy is the Overridden=True reason used
	// when BackendConfigPolicy.spec.tls is dropped because another
	// BackendConfigPolicy with higher weight, or older at equal weight, supplies it.
	ReasonConflictedWithBackendConfigPolicy = "ConflictedWithBackendConfigPolicy"
)
