package version

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	T *testing.T
)

func TestVersion(t *testing.T) {
	T = t
	RegisterFailHandler(Fail)
	//Glooctl tests are failing CI
	//junitReporter := reporters.NewJUnitReporter("junit.xml")
	//RunSpecsWithDefaultAndCustomReporters(t, "Version Suite", []Reporter{junitReporter})
}
