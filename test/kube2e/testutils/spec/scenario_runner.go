package spec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
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
//
// - // TODO: add a test to demonstrate that does not break
type ScenarioRunner struct {
	progressWriter io.Writer
	timeout        time.Duration
}

// NewScenarioRunner returns a ScenarioRunner
func NewScenarioRunner() *ScenarioRunner {
	return &ScenarioRunner{
		progressWriter: io.Discard,
	}
}

// WithProgressWriter sets the io.Writer used by the ScenarioRunner
func (s *ScenarioRunner) WithProgressWriter(writer io.Writer) *ScenarioRunner {
	s.progressWriter = writer
	return s
}

// WithTimeout sets the maximum time that a Scenario will be run for
func (s *ScenarioRunner) WithTimeout(timeout time.Duration) *ScenarioRunner {
	s.timeout = timeout
	return s
}

// RunScenario executes a Scenario
func (s *ScenarioRunner) RunScenario(ctx context.Context, scenario Scenario) error {
	scenarioCtx, scenarioCancel := context.WithTimeout(ctx, s.timeout)
	defer scenarioCancel()

	return s.runScenarioRecursive(scenarioCtx, scenario, 1)
}

func (s *ScenarioRunner) runScenarioRecursive(ctx context.Context, scenario Scenario, currentScenarioDepth int) (err error) {
	if scenario == nil {
		return nil
	}

	if currentScenarioDepth > maxScenarioDepth {
		return errors.New("scenario can be composed of one another, but 3 levels is the maximum")
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
		err = s.cleanupScenario(ctx, scenario)
	}()

	s.writeProgress(scenario, "running assertion")
	scenario.Assertion()(ctx)

	s.writeProgress(scenario, "running child scenarios")
	if childScenarioErr := s.runScenarioRecursive(ctx, scenario.ChildScenario(), currentScenarioDepth+1); childScenarioErr != nil {
		return childScenarioErr
	}

	return err
}

func (s *ScenarioRunner) setupScenario(ctx context.Context, scenario Scenario) error {
	err := scenario.InitializeResources()(ctx)
	if err != nil {
		return err
	}

	scenario.WaitForInitialized()(ctx)
	return nil
}

func (s *ScenarioRunner) cleanupScenario(ctx context.Context, scenario Scenario) error {
	err := scenario.FinalizeResources()(ctx)
	if err != nil {
		return err
	}

	scenario.WaitForFinalized()(ctx)
	return nil
}

func (s *ScenarioRunner) writeProgress(scenario Scenario, progress string) {
	_, _ = s.progressWriter.Write([]byte(fmt.Sprintf("%s: %s\n", scenario.Name(), progress)))
}
