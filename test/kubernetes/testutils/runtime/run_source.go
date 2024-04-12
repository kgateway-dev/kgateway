package runtime

// RunSource identifies who/what triggered the test
type RunSource int

// Declare related constants for each direction starting with index 1
const (
	// LocalDevelopment signifies that the test is invoked locally
	LocalDevelopment RunSource = iota + 1 // EnumIndex = 1

	// PullRequest means that the test was invoked while running CI against a Pull Request
	PullRequest // EnumIndex = 2

	// NightlyTest means that the test was invoked while running CI as part of a Nightly operation
	NightlyTest // EnumIndex = 3
)
