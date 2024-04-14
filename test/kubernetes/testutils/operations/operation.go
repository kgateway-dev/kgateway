package operations

import (
	"context"

	"github.com/solo-io/gloo/test/kubernetes/testutils/actions"

	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
)

// Operation defines the properties of an operation that can be applied to a Kubernetes cluster
// An Operation is intended to be simple, and encapsulate two concepts:
//
//	Action - A mutation that is applied to a cluster
//	Assertion - An assertion that the mutation behaved appropriately
type Operation interface {
	// Name returns the name of the operation
	Name() string

	// Action returns the actions.ClusterAction that will be executed against the cluster
	// This is a function that mutates state on the cluster
	Action() actions.ClusterAction

	// Assertion returns the assertions.ClusterAssertion that will run after the Action is executed
	// This is a function that asserts behavior of the cluster
	Assertion() assertions.ClusterAssertion
}

// ReversibleOperation combines two Operation, that are the inverse of one another
// We recommend that developers write tests using ReversibleOperation.
// This is because when these are executed, they leave the cluster in the state they found it.
// If resources are not cleaned up properly, that can lead to pollution in the cluster and test flakes
type ReversibleOperation struct {
	Do   Operation
	Undo Operation
}

var _ Operation = new(BasicOperation)

// BasicOperation is an implementation of the Operation interface, with the minimal properties required
type BasicOperation struct {
	OpName       string
	OpAction     func(ctx context.Context) error
	OpAssertion  assertions.ClusterAssertion
	OpAssertions []assertions.ClusterAssertion
}

func (o *BasicOperation) Name() string {
	return o.OpName
}

func (o *BasicOperation) Action() actions.ClusterAction {
	return o.OpAction
}

func (o *BasicOperation) Assertion() assertions.ClusterAssertion {
	return func(ctx context.Context) {
		for _, assertion := range append(o.OpAssertions, o.OpAssertion) {
			if assertion != nil {
				assertion(ctx)
			}
		}
	}
}
