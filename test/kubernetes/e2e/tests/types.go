package tests

import (
	"context"
	"testing"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

type (
	TestGenerator func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T)
	TestFunc      func(t *testing.T)
	RunFunc       func(name string, test TestFunc)
	NewSuiteFunc  func(ctx context.Context, testInstallation *e2e.TestInstallation) suite.TestingSuite

	NamedTest struct {
		Name string
		Test NewSuiteFunc
	}

	OrderedTests []NamedTest

	UnorderedTests map[string]NewSuiteFunc

	TestRunner interface {
		Run(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation)
		Register(name string, newSuite NewSuiteFunc)
	}
)

var (
	_ TestRunner = new(OrderedTests)
	_ TestRunner = new(UnorderedTests)
)

func (o OrderedTests) Run(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) {
	for _, namedTest := range o {
		t.Run(namedTest.Name, func(t *testing.T) { namedTest.Test(ctx, testInstallation) })
	}
}

func (o OrderedTests) Register(name string, newSuite NewSuiteFunc) {
	o = append(o, NamedTest{
		Name: name,
		Test: newSuite,
	})

}

func (u UnorderedTests) Run(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) {
	// TODO(jbohanon) does some randomness need to be injected here to ensure they aren't run in the same order every time?
	// from https://goplay.tools/snippet/A-qqQCWkFaZ it looks like maps are not stable, but tend toward stability.
	for testName, test := range u {
		t.Run(testName, func(t *testing.T) { test(ctx, testInstallation) })
	}
}

func (u UnorderedTests) Register(name string, newSuite NewSuiteFunc) {
	u[name] = newSuite
}
