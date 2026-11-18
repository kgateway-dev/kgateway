package conditions

const (

	// This condition indicates whether a route has generated some
	// configuration that will soon be ready in the underlying data plane.
	KgatewayConditionProgrammed = "kgateway.dev/Programmed"

	// This reason is used with the "kgateway.dev/Programmed" condition when
	// the condition is true.
	KgatewayReasonProgrammed = "Programmed"

	// This reason is used with the "kgateway.dev/Programmed" condition when
	// the condition is true, but a route rule's sessionPersistence sets an
	// absoluteTimeout that has no effect: it only applies to Cookie sessions
	// with a Permanent cookie lifetimeType. The route is still programmed,
	// without a session lifetime.
	KgatewayReasonSessionPersistenceAbsoluteTimeoutIgnored = "SessionPersistenceAbsoluteTimeoutIgnored"

	// This reason is used with the "kgateway.dev/Programmed" condition when
	// the condition is true, but a route rule's sessionPersistence is
	// overridden by the sessionPersistence of a BackendConfigPolicy attached
	// to one or more of the rule's backends. The route is still programmed,
	// with the BackendConfigPolicy's session persistence for those backends.
	KgatewayReasonSessionPersistenceOverridden = "SessionPersistenceOverridden"

	// This reason is used with the "kgateway.dev/Programmed" condition when
	// the condition is true, but one or more entries of the route's
	// kgateway.dev/http-redirect-status-code annotation are not used by any
	// rule, for example because no rule has the name an entry is scoped to.
	// The route is still programmed, without those entries.
	KgatewayReasonHTTPRedirectStatusCodeIgnored = "HTTPRedirectStatusCodeIgnored"
)
