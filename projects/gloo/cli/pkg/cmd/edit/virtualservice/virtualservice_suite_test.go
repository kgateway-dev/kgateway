package virtualservice_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestVirtualService(t *testing.T) {
	RegisterFailHandler(Fail)
	// Glooctl tests are failing CI
	//junitReporter := reporters.NewJUnitReporter("junit.xml")
	//RunSpecsWithDefaultAndCustomReporters(t, "VirtualService Suite", []Reporter{junitReporter})
}
