package e2e

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/provider"
)

const (
	// ctxKey is the name of the key used to store Suite details
	ctxKey = "SuiteContext"
)

type SuiteContext struct {
	Operator           *operations.Operator
	OperationsProvider *provider.OperationProvider
	AssertionProvider  *assertions.Provider
}

func Store(specCtx SpecContext, suiteCtx *SuiteContext) {
	context.WithValue(specCtx, ctxKey, suiteCtx)
}

func SuiteDescribe(text string, specFn func(ctx *SuiteContext)) bool {
	Describe(text, Offset(1), Ordered, func() {

		var (
			suiteCtx *SuiteContext
		)

		BeforeAll(func(specContext SpecContext) {
			suiteContext, ok := specContext.Value(ctxKey).(*SuiteContext)
			Expect(ok).To(BeTrue())

			suiteCtx = suiteContext
		})

		specFn(suiteCtx)

	})
	return true
}
