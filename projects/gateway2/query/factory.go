package query

import (
	"context"

	controllerruntime "sigs.k8s.io/controller-runtime"
)

// EngineFactory is a factory function to produce query.Engine implementations
type EngineFactory func(ctx context.Context, manager controllerruntime.Manager) Engine

// Engine exposes a series of queries that the Control Plane can execute to access Kubernetes objects
// An Engine relies on a series of Indexers to optimize the caching used by the shared informers
type Engine interface {
	GatewayQueries
}

// GetEngineFactory returns the implementation of the Gloo Gateway Open Source EngineFactory
func GetEngineFactory() EngineFactory {
	return func(ctx context.Context, manager controllerruntime.Manager) Engine {
		return NewData(manager.GetClient(), manager.GetScheme())
	}
}
