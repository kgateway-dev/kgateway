package annotations

const (
	// RoutePrecedenceWeight is an annotation that can be set on an HTTPRoute to specify the weight of
	// the route as an integer value (negative values are allowed).
	// Routes with higher weight implies higher priority, and are evaluated before routes with lower weight.
	// By default, routes have a weight of 0.
	RoutePrecedenceWeight = "kgateway.dev/route-weight"
)
