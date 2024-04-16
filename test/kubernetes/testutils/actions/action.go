package actions

import "context"

// ClusterAction is a function that will be executed against the cluster
// If it succeeds, it will not return anything
// If it fails, it will return an error
// A ClusterAction must not panic! It is intended to be a function that can fail
type ClusterAction func(ctx context.Context) error
