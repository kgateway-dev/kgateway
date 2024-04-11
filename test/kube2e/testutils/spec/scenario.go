package spec

import "context"

// ScenarioAssertion is a function which asserts a given behavior
// If it succeeds, it will not return anything
// If it fails, it will panic
type ScenarioAssertion func(ctx context.Context)

// Scenario defines the properties of a test scenario
type Scenario interface {
	// Name returns the name of the scenario
	Name() string

	// InitializeResources returns the function that will create any resources that the Scenario used
	InitializeResources() func(ctx context.Context) error

	// WaitForInitialized returns the ScenarioAssertion that must pass before the Scenario can proceed
	WaitForInitialized() ScenarioAssertion

	// Assertion returns the ScenarioAssertion that will run during the Scenario
	Assertion() ScenarioAssertion

	// ChildScenario returns a Scenario that will be run during the scenario, after the Assertion() is executed
	// This is an optional property, that allows nesting of Scenario
	ChildScenario() Scenario

	// FinalizeResources returns the function that will remove any resources that the Scenario used
	FinalizeResources() func(ctx context.Context) error

	// WaitForFinalized returns the ScenarioAssertion that must pass before the Scenario is completed
	WaitForFinalized() ScenarioAssertion
}
