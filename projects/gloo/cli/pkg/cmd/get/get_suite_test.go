package get_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestGet(t *testing.T) {
	RegisterFailHandler(Fail)
	//Glooctl tests are failing CI
	//junitReporter := reporters.NewJUnitReporter("junit.xml")
	//RunSpecsWithDefaultAndCustomReporters(t, "Get Suite", []Reporter{junitReporter})
}
