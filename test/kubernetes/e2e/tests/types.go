package tests

import (
	"context"
	"testing"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
)

type (
	TestGenerator func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T)

	NamedTest struct {
		Name string
		Test TestGenerator
	}

	OrderedTests []NamedTest

	UnorderedTests map[string]TestGenerator

	TestRunner interface {
		Run(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation)
	}
)

var (
	_ TestRunner = new(OrderedTests)
	_ TestRunner = new(UnorderedTests)
)

func (o OrderedTests) Run(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) {
	for _, namedTest := range o {
		t.Run(namedTest.Name, namedTest.Test(ctx, t, testInstallation))
	}
}

func (u UnorderedTests) Run(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) {
	// TODO(jbohanon) does some randomness need to be injected here to ensure they aren't run in the same order every time?
	// from https://goplay.tools/snippet/A-qqQCWkFaZ it looks like maps are not stable, but tend toward stability.
	for testName, test := range u {
		t.Run(testName, test(ctx, t, testInstallation))
	}
}
