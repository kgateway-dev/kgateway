package operations

import "context"

// DiscreteAssertion is a function which asserts a given behavior at a point in time
// If it succeeds, it will not return anything
// If it fails, it will panic
type DiscreteAssertion func(ctx context.Context)

// Operation defines the properties of an operation that can be applied to a Kubernetes cluster
type Operation interface {
	// Name returns the name of the operation
	Name() string

	Execute() func(ctx context.Context) error

	// ExecutionAssertion returns the DiscreteAssertion that will run after the Operation is executed
	ExecutionAssertion() DiscreteAssertion
}

// ReversibleOperation combines two Operation, that are the inverse of one another
type ReversibleOperation struct {
	Do   Operation
	Undo Operation
}
