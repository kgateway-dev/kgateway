package operations_test

import (
	"context"
	"strconv"
	"strings"

	"github.com/solo-io/gloo/test/kubernetes/testutils/actions"

	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
)

var _ = Describe("Operator", func() {

	var (
		ctx      context.Context
		operator *operations.Operator
	)

	BeforeEach(func() {
		ctx = context.Background()
		operator = operations.NewGinkgoOperator()
	})

	Context("ExecuteOperations", func() {

		It("does not return error for valid operations", func() {
			operation := &testOperation{
				op: func(ctx context.Context) error {
					return nil
				},
				assertion: func(ctx context.Context) {
					Expect(1).To(Equal(1), "one does equal one")
				},
			}

			err := operator.ExecuteOperations(ctx, operation)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error on operation.Action", func() {
			operation := &testOperation{
				op: func(ctx context.Context) error {
					return eris.Errorf("Failed to execute operation")
				},
				assertion: func(ctx context.Context) {
					Expect(1).To(Equal(1), "one does equal one")
				},
			}

			err := operator.ExecuteOperations(ctx, operation)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("Failed to execute operation"))
		})

		It("returns error if assertion fails", func() {
			operation := &testOperation{
				op: func(ctx context.Context) error {
					return nil
				},
				assertion: func(ctx context.Context) {
					Expect(1).To(Equal(2), "one does not equal two")
				},
			}

			err := operator.ExecuteOperations(ctx, operation)
			Expect(err).To(And(
				// Prove that the error includes the description of the failing assertion
				MatchError(ContainSubstring("one does not equal")),
				// Prove that the error includes the assertion that failed
				MatchError(ContainSubstring("Expected\n    <int>: 1\nto equal\n    <int>: 2")),
			))
		})

	})

	Context("ExecuteReversibleOperations", func() {

		It("does not return error for valid operations", func() {
			var (
				order []string
				ops   []operations.ReversibleOperation
			)

			for i := 1; i <= 3; i++ {
				iStr := strconv.Itoa(i)
				op := operations.ReversibleOperation{
					Do: &testOperation{
						op: func(ctx context.Context) error {
							order = append(order, "+", iStr)
							return nil
						},
					},
					Undo: &testOperation{
						op: func(ctx context.Context) error {
							order = append(order, "-", iStr)
							return nil
						},
					},
				}

				ops = append(ops, op)
			}

			err := operator.ExecuteReversibleOperations(ctx, ops...)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.Join(order, "")).To(Equal("+1+2+3-3-2-1"), "operations are executed in proper order")
		})

		It("returns error if operation fails", func() {
			var (
				order []string
				ops   []operations.ReversibleOperation
			)

			for i := 1; i <= 3; i++ {
				iStr := strconv.Itoa(i)
				op := operations.ReversibleOperation{
					Do: &testOperation{
						op: func(ctx context.Context) error {
							order = append(order, "+", iStr)
							return nil
						},
						assertion: func(ctx context.Context) {
							// This will only fail for one of the Operation
							Expect(iStr).NotTo(Equal("2"), "iStr is 2")
						},
					},
					Undo: &testOperation{
						op: func(ctx context.Context) error {
							order = append(order, "-", iStr)
							return nil
						},
					},
				}

				ops = append(ops, op)
			}

			err := operator.ExecuteReversibleOperations(ctx, ops...)
			Expect(err).To(MatchError(ContainSubstring("iStr is 2")))
			Expect(strings.Join(order, "")).To(Equal("+1+2"), "operations are executed in proper order, even on failure")
		})

	})

})

var _ operations.Operation = new(testOperation)

// testOperation is used only in this file, to validate that the Operator behaves as expected
type testOperation struct {
	name      string
	op        actions.ClusterAction
	assertion assertions.ClusterAssertion
}

func (t *testOperation) String() string {
	if t.name != "" {
		return t.name
	}
	return "test-operation"
}

func (t *testOperation) Action() actions.ClusterAction {
	if t.op != nil {
		return t.op
	}
	return func(ctx context.Context) error {
		return nil
	}
}

func (t *testOperation) Assertion() assertions.ClusterAssertion {
	if t.assertion != nil {
		return t.assertion
	}
	return func(ctx context.Context) {
		// no nothing
	}
}
