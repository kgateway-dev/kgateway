package consul

import (
	"testing"

	"github.com/solo-io/gloo/test/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConsul(t *testing.T) {
	// This has caused issues when tests are run in parallel (not enabled in CI)
	leakDetector := helpers.DeferredGoroutineLeakDetector(t)
	defer leakDetector()

	RegisterFailHandler(Fail)
	RunSpecs(t, "Consul Plugin Suite")
}
