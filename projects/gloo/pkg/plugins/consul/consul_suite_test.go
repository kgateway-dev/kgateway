package consul

import (
	"github.com/solo-io/gloo/test/helpers"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
)

var T *testing.T

func TestConsul(t *testing.T) {
	leakDetector := helpers.DeferredGoroutineLeakDetector(t)
	defer leakDetector()
	RegisterFailHandler(Fail)
	T = t
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Consul Plugin Suite", []Reporter{junitReporter})
}
