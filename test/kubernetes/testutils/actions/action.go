package actions

import "context"

// ClusterAction is a a function that will be executed against the cluster
// If it succeeds, it will not return anything
// If it fails, it will return an error
type ClusterAction func(ctx context.Context) error
