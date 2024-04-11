package spec

import (
	"context"
	"fmt"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	"io"
)

const (
	// maxScenarioDepth represents the number of Scenario that can be nested within one another
	// This number is "magic" in that it was chosen as a value allows for the most common Scenario
	// to be defined, while also preventing any infinite loops from being introduced
	// If you find that you define a Scenario whose depth exceed maxScenarioDepth you should either:
	//	1. Evaluate the test, and see if you should simplify it
	//	2. Increase this value
	maxScenarioDepth = 3
)

// ScenarioRunner is responsible for running Scenario
// Expectations of a ScenarioRunner are:
//   - On failure, a runner should have a configurable way to leave resources behind if you want
//   - A runner should be able to run a scenario multiple times in a row
type ScenarioRunner struct {
	progressWriter       io.Writer
	assertionInterceptor func(func()) error
}

// NewScenarioRunner returns a ScenarioRunner
func NewScenarioRunner() *ScenarioRunner {
	return &ScenarioRunner{
		progressWriter: io.Discard,
		assertionInterceptor: func(f func()) error {
			// do nothing, assertions will bubble up and panic
			return nil
		},
	}
}

// NewGinkgoScenarioRunner returns a ScenarioRunner used for the Ginkgo test framework
func NewGinkgoScenarioRunner() *ScenarioRunner {
	return NewScenarioRunner().
		WithProgressWriter(ginkgo.GinkgoWriter).
		WithAssertionInterceptor(gomega.InterceptGomegaFailure)
}

// WithProgressWriter sets the io.Writer used by the ScenarioRunner
func (s *ScenarioRunner) WithProgressWriter(writer io.Writer) *ScenarioRunner {
	s.progressWriter = writer
	return s
}

// WithAssertionInterceptor sets the function that will be used to intercept ScenarioAssertion failures
func (s *ScenarioRunner) WithAssertionInterceptor(assertionInterceptor func(func()) error) *ScenarioRunner {
	s.assertionInterceptor = assertionInterceptor
	return s
}

// RunScenario executes a Scenario
func (s *ScenarioRunner) RunScenario(ctx context.Context, scenario Scenario) error {
	// Intercept failures, so that we can return an error to the test code,
	// and it can decide what to do with it
	var scenarioErr error
	interceptedErr := s.assertionInterceptor(func() {
		scenarioErr = s.runScenarioRecursive(ctx, scenario, 1)
	})
	if interceptedErr != nil {
		return interceptedErr
	}
	return scenarioErr
}

func (s *ScenarioRunner) runScenarioRecursive(ctx context.Context, scenario Scenario, currentScenarioDepth int) (err error) {
	if scenario == nil {
		return nil
	}

	if currentScenarioDepth > maxScenarioDepth {
		return eris.Errorf("scenario can be nested, but %d levels is the maximum", maxScenarioDepth)
	}

	s.writeProgress(scenario, "running setup")
	if setupErr := s.setupScenario(ctx, scenario); setupErr != nil {
		return setupErr
	}

	defer func() {
		// We rely on a defer function to handle the cleanup, to ensure that we ALWAYS perform cleanup
		// https://go.dev/blog/defer-panic-and-recover
		// An assertion within the Scenario may cause the test itself to panic, and we want to ensure
		// that no resources are left behind in the cluster, after a Scenario runs
		s.writeProgress(scenario, "running cleanup")
		cleanupErr := s.cleanupScenario(ctx, scenario)
		if cleanupErr != nil {
			err = cleanupErr
		}
	}()

	s.writeProgress(scenario, "running assertion")
	scenario.Assertion()(ctx)

	s.writeProgress(scenario, "running child scenarios")
	return s.runScenarioRecursive(ctx, scenario.ChildScenario(), currentScenarioDepth+1)
}

func (s *ScenarioRunner) setupScenario(ctx context.Context, scenario Scenario) error {
	err := scenario.InitializeResources()(ctx)
	if err != nil {
		return err
	}

	scenario.InitializedAssertion()(ctx)
	return nil
}

func (s *ScenarioRunner) cleanupScenario(ctx context.Context, scenario Scenario) error {
	err := scenario.FinalizeResources()(ctx)
	if err != nil {
		return err
	}

	scenario.FinalizedAssertion()(ctx)
	return nil
}

func (s *ScenarioRunner) writeProgress(scenario Scenario, progress string) {
	_, _ = s.progressWriter.Write([]byte(fmt.Sprintf("%s: %s\n", scenario.Name(), progress)))
}
