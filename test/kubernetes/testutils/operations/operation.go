package operations

import (
	"context"

	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
)

// Operation defines the properties of an operation that can be applied to a Kubernetes cluster
type Operation interface {
	// Name returns the name of the operation
	Name() string

	Execute() func(ctx context.Context) error

	// ExecutionAssertion returns the DiscreteAssertion that will run after the Operation is executed
	ExecutionAssertion() assertions.DiscreteAssertion
}

// ReversibleOperation combines two Operation, that are the inverse of one another
type ReversibleOperation struct {
	Do   Operation
	Undo Operation
}
