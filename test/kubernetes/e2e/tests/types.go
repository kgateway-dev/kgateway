package tests

import (
	"context"
	"testing"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

type (
	NamedTest struct {
		Name     string
		NewSuite e2e.NewSuiteFunc
	}

	OrderedTests struct {
		tests []NamedTest
	}

	UnorderedTests struct {
		tests map[string]e2e.NewSuiteFunc
	}

	TestRunner interface {
		Run(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation)
		Register(name string, newSuite e2e.NewSuiteFunc)
	}
)

var (
	_ TestRunner = new(OrderedTests)
	_ TestRunner = new(UnorderedTests)
)

func (o OrderedTests) Run(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) {
	for _, namedTest := range o.tests {
		t.Run(namedTest.Name, func(t *testing.T) {
			suite.Run(t, namedTest.NewSuite(ctx, testInstallation))
		})
	}
}

func (o *OrderedTests) Register(name string, newSuite e2e.NewSuiteFunc) {
	if o.tests == nil {
		o.tests = make([]NamedTest, 0)
	}
	o.tests = append(o.tests, NamedTest{
		Name:     name,
		NewSuite: newSuite,
	})

}

func (u UnorderedTests) Run(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) {
	// TODO(jbohanon) does some randomness need to be injected here to ensure they aren't run in the same order every time?
	// from https://goplay.tools/snippet/A-qqQCWkFaZ it looks like maps are not stable, but tend toward stability.
	for testName, newSuite := range u.tests {
		t.Run(testName, func(t *testing.T) {
			suite.Run(t, newSuite(ctx, testInstallation))
		})
	}
}

func (u *UnorderedTests) Register(name string, newSuite e2e.NewSuiteFunc) {
	if u.tests == nil {
		u.tests = make(map[string]e2e.NewSuiteFunc)
	}
	u.tests[name] = newSuite
}
