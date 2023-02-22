package consul

import (
	"testing"

	"github.com/solo-io/gloo/test/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var T *testing.T

func TestConsul(t *testing.T) {
	leakDetector := helpers.DeferredGoroutineLeakDetector(t)
	defer leakDetector()

	RegisterFailHandler(Fail)
	T = t
	RunSpecs(t, "Consul Plugin Suite")
}
