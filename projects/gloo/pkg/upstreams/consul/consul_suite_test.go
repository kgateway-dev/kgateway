package consul

import (
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
)

var T *testing.T

func TestConsul(t *testing.T) {
	RegisterFailHandler(Fail)
	T = t
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Consul Upstream Client Suite", []Reporter{junitReporter})
}
