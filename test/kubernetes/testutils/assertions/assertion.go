package assertions

import "context"

// ClusterAssertion is a function which asserts a given behavior at a point in time
// If it succeeds, it will not return anything
// If it fails, it will panic
type ClusterAssertion func(ctx context.Context)
