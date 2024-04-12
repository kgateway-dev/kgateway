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

	// ExecutionAssertion returns the assertions.DiscreteAssertion that will run after the Operation is executed
	ExecutionAssertion() assertions.DiscreteAssertion
}

// ReversibleOperation combines two Operation, that are the inverse of one another
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
