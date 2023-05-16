package labels

const (
	// Nightly is a label applied to any tests which should run during our nightly tests and not during PRs
	Nightly = "nightly"

	// Performance is a label applied to any tests which run performance tests
	// These often require more resources/time to complete, and likely report their findings to a remote location
	Performance = "performance"
)
