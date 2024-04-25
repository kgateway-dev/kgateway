package operations

import (
	"context"
	"fmt"
	"io"
	"slices"

	errors "github.com/rotisserie/eris"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// Operator is responsible for executing Operation against a Kubernetes Cluster.
// This is meant to mirror the behavior of a user of the Gloo Gateway product.
// Although we operate against a Kubernetes Cluster, an Operator is intentionally
// unaware of Kubernetes behavior, and instead is more of a scheduler of Operation.
// This allows us to test its functionality, and also more easily inject behaviors
type Operator struct {
	progressWriter       io.Writer
	assertionInterceptor func(func()) error
}

// NewOperator returns an Operator
func NewOperator() *Operator {
	return &Operator{
		progressWriter: io.Discard,
		assertionInterceptor: func(f func()) error {
			// do nothing, assertions will bubble up and lead to a panic
			return nil
		},
	}
}

// NewGinkgoOperator returns an Operator used for the Ginkgo test framework
func NewGinkgoOperator() *Operator {
	return NewOperator().
		WithProgressWriter(ginkgo.GinkgoWriter).
		WithAssertionInterceptor(gomega.InterceptGomegaFailure)
}

// WithProgressWriter sets the io.Writer used by the Operator
func (o *Operator) WithProgressWriter(writer io.Writer) *Operator {
	o.progressWriter = writer
	return o
}

// WithAssertionInterceptor sets the function that will be used to intercept ScenarioAssertion failures
func (o *Operator) WithAssertionInterceptor(assertionInterceptor func(func()) error) *Operator {
	o.assertionInterceptor = assertionInterceptor
	return o
}

// ExecuteOperations executes a set of Operation.
// NOTE: The Operator doesn't attempt to undo any of these Operation so if you are modifying
// resources on the Cluster, it is your responsibility to perform Operation to undo those changes
// If you would like to rely on this functionality, please see ExecuteReversibleOperations.
func (o *Operator) ExecuteOperations(ctx context.Context, operations ...Operation) error {
	return o.executeSafe(func() error {
		return o.executeOperations(ctx, operations...)
	})
}

// ExecuteReversibleOperations executes a set of ReversibleOperation.
// In order, the ReversibleOperation.Do will be executed, and then on success or failure
// the ReversibleOperation.Undo will also be executed.
// This way, developers do not need to worry about resources being cleaned up appropriately in tests.
func (o *Operator) ExecuteReversibleOperations(ctx context.Context, operations ...ReversibleOperation) error {
	return o.executeSafe(func() error {
		return o.executeReversibleOperations(ctx, operations...)
	})
}

func (o *Operator) executeSafe(fnMayPanic func() error) error {
	// Intercept failed assertions, which manifests as Panic's in test frameworks
	// This way, we can return an error to the testing code, and let the test author decide how to manage it
	var executionErr error
	interceptedErr := o.assertionInterceptor(func() {
		executionErr = fnMayPanic()
	})
	if interceptedErr != nil {
		return interceptedErr
	}
	return executionErr
}

func (o *Operator) executeOperations(ctx context.Context, operations ...Operation) error {
	for _, op := range operations {
		if err := o.executeOperation(ctx, op); err != nil {
			return err
		}
	}
	return nil
}

func (o *Operator) executeReversibleOperations(ctx context.Context, operations ...ReversibleOperation) (err error) {
	var undoOperations []Operation

	for _, op := range operations {
		undoOperations = append(undoOperations, op.Undo)

		doErr := o.executeOperation(ctx, op.Do)
		if doErr != nil {
			return doErr
		}
	}

	// We need to perform the undo operations in reverse order
	// This way, if we execute: do-A -> do-B -> do-C
	// We should undo it by executing: undo-C -> undo-B -> undo-A
	slices.Reverse(undoOperations)

	// NOTE TO DEVELOPERS: In the past, we would perform this in a deferred function so that
	// it would always execute. This made debugging more challenging if an assertion failed,
	// because the undo operations would always run, and modify the cluster
	//
	// TODO: Help Wanted
	// We may want to improve this further, and make the cleanup strategy configurable.
	// If I am writing a test, and it fails locally, I may still want the operation to be undo afterwards
	return o.executeOperations(ctx, undoOperations...)
}

func (o *Operator) executeOperation(ctx context.Context, operation Operation) error {
	o.writeProgress(operation, "starting operation")

	action := operation.Action()
	o.writeProgress(operation, "executing operation")
	if err := action(ctx); err != nil {
		return err
	}

	o.writeProgress(operation, "asserting operation")
	assertion := operation.Assertion()
	if assertion == nil {
		// We want to make it impossible for developers to accidentally define operations that do not assert any behavior
		// If a developer wants to provide a no-op implementation, they will, but this check ensures that it is intentional
		return errors.Errorf("Operation (%s) contained a nil assertion, which is not allowed", operation)
	}

	assertion(ctx)
	o.writeProgress(operation, "completing operation")
	return nil
}

func (o *Operator) writeProgress(operation Operation, progress string) {
	o.Logf("%s (%s)", progress, operation)
}

func (o *Operator) Logf(messageFmt string, args ...any) {
	message := fmt.Sprintf(messageFmt, args...)
	_, _ = o.progressWriter.Write([]byte(fmt.Sprintf("OPERATOR: %s \n", message)))
}
