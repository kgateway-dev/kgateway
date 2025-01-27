package decorators

import "github.com/onsi/ginkgo/v2"

// Ginkgo Decorators (https://onsi.github.io/ginkgo/#decorator-reference)

const (
	// Vault is a decorator that allows you to mark a spec or container as requiring our Vault service (test/services/vault.go)
	// The Vault service uses a hard-coded port, so only one test can use it at a time.
	Vault = ginkgo.Serial

	// Consul is a decorator that allows you to mark a spec or container as requiring our Consul service (test/services/consul.go)
	// The Consul service uses on a hard-coded port for the Consul agent (8300)
	Consul = ginkgo.Serial

	// Performance is a decorator that allows you to mark a spec or container as running performance tests
	// These often require more resources/time to complete, and for consistency, should not be run in parallel with other tests
	Performance = ginkgo.Serial
)
