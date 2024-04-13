package operations

import (
	"context"

	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
)

// Operation defines the properties of an operation that can be applied to a Kubernetes cluster
// An Operation is intended to be simple, and mirror the action that a user would perform
type Operation interface {
	// Name returns the name of the operation
	Name() string

	// Execute returns the function that will be executed against the cluster
	Execute() func(ctx context.Context) error

	// ExecutionAssertion returns the assertions.DiscreteAssertion that will run after the Operation is executed
	ExecutionAssertion() assertions.DiscreteAssertion
}

// ReversibleOperation combines two Operation, that are the inverse of one another
// We recommend that developers write tests using ReversibleOperation
// This is because when these are executed, they leave the cluster in the state they found it
// If resources are not cleaned up properly, that can lead to pollution in the cluster and test flakes
type ReversibleOperation struct {
	Do   Operation
	Undo Operation
}

var _ Operation = new(BasicOperation)

// BasicOperation is an implementation of the Operation interface, with the minimal properties required
type BasicOperation struct {
	OpName       string
	OpExecute    func(ctx context.Context) error
	OpAssertions []assertions.DiscreteAssertion
}

func (o *BasicOperation) Name() string {
	return o.OpName
}

func (o *BasicOperation) Execute() func(ctx context.Context) error {
	return o.OpExecute
}

func (o *BasicOperation) ExecutionAssertion() assertions.DiscreteAssertion {
	return func(ctx context.Context) {
		for _, ast := range o.OpAssertions {
			ast(ctx)
		}
	}
}
