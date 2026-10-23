package conditions

const (

	// This condition indicates whether a route has generated some
	// configuration that will soon be ready in the underlying data plane.
	KgatewayConditionProgrammed = "kgateway.dev/Programmed"

	// This reason is used with the "kgateway.dev/Programmed" condition when
	// the condition is true.
	KgatewayReasonProgrammed = "Programmed"

	// This reason is used with the "kgateway.dev/Programmed" condition when
	// the condition is true, but a route rule's sessionPersistence is
	// overridden by the sessionPersistence of a BackendConfigPolicy attached
	// to one or more of the rule's backends. The route is still programmed,
	// with the BackendConfigPolicy's session persistence for those backends.
	KgatewayReasonSessionPersistenceOverridden = "SessionPersistenceOverridden"
)
