package labels

// Ginkgo Spec Labels (https://onsi.github.io/ginkgo/#spec-labels)

const (
	// Performance is a label applied to any tests which run performance tests
	// These often require more resources/time to complete, and likely report their findings to a remote location
	Performance = "performance"

	// E2E is a label applied to any tests which run Gloo Edge end-to-end
	E2E = "end-to-end"
)
