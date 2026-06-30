package conditions

const (

	// This condition indicates whether a route has generated some
	// configuration that will soon be ready in the underlying data plane.
	KgatewayConditionProgrammed = "kgateway.dev/Programmed"

	// This condition indicates that the controller was unable to resolve
	// conflicting specification requirements for this route.
	KgatewayConditionConflicted = "kgateway.dev/Conflicted"

	// This reason is used with the "kgateway.dev/Programmed" condition when
	// the condition is true.
	KgatewayReasonProgrammed = "Programmed"
)
