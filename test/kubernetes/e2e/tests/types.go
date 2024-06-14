package tests

import (
	"context"
	"testing"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

type (
	namedSuite struct {
		Name     string
		NewSuite e2e.NewSuiteFunc
	}

	orderedTests struct {
		tests []namedSuite
	}

	tests struct {
		tests map[string]e2e.NewSuiteFunc
	}

	// A TestRunner is an interface that allows E2E tests to simply Register tests in one location and execute them
	// with Run.
	TestRunner interface {
		Run(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation)
		Register(name string, newSuite e2e.NewSuiteFunc)
	}
)

var (
	_ TestRunner = new(orderedTests)
	_ TestRunner = new(tests)
)

// NewTestRunner returns an implementation of TestRunner that will execute tests as specified
// in the ordered parameter.
//
// NOTE: it should be strongly preferred to use unordered tests. Only pass true to this function
// if there is a clear need for the tests to be ordered, and specify in a comment near the call
// to NewTestRunner why the tests need to be ordered.
func NewTestRunner(ordered bool) TestRunner {
	if ordered {
		return new(orderedTests)
	}

	return new(tests)
}

func (o orderedTests) Run(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) {
	for _, namedTest := range o.tests {
		t.Run(namedTest.Name, func(t *testing.T) {
			suite.Run(t, namedTest.NewSuite(ctx, testInstallation))
		})
	}
}

func (o *orderedTests) Register(name string, newSuite e2e.NewSuiteFunc) {
	if o.tests == nil {
		o.tests = make([]namedSuite, 0)
	}
	o.tests = append(o.tests, namedSuite{
		Name:     name,
		NewSuite: newSuite,
	})

}

func (u tests) Run(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) {
	// TODO(jbohanon) does some randomness need to be injected here to ensure they aren't run in the same order every time?
	// from https://goplay.tools/snippet/A-qqQCWkFaZ it looks like maps are not stable, but tend toward stability.
	for testName, newSuite := range u.tests {
		t.Run(testName, func(t *testing.T) {
			suite.Run(t, newSuite(ctx, testInstallation))
		})
	}
}

func (u *tests) Register(name string, newSuite e2e.NewSuiteFunc) {
	if u.tests == nil {
		u.tests = make(map[string]e2e.NewSuiteFunc)
	}
	u.tests[name] = newSuite
}
