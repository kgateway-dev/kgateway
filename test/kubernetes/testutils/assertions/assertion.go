package assertions

import "context"

// DiscreteAssertion is a function which asserts a given behavior at a point in time
// If it succeeds, it will not return anything
// If it fails, it will panic
type DiscreteAssertion func(ctx context.Context)
