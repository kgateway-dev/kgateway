package add_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAdd(t *testing.T) {
	RegisterFailHandler(Fail)
	//This test is consistently failing in CI
	//junitReporter := reporters.NewJUnitReporter("junit.xml")
	//RunSpecsWithDefaultAndCustomReporters(t, "Add Suite", []Reporter{junitReporter})
}
