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
)
