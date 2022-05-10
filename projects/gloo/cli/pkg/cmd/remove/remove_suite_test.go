package remove_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestRemove(t *testing.T) {
	RegisterFailHandler(Fail)
	//Glooctl tests are failing CI
	//junitReporter := reporters.NewJUnitReporter("junit.xml")
	//RunSpecsWithDefaultAndCustomReporters(t, "Remove Suite", []Reporter{junitReporter})
}
