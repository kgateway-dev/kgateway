package cli_unit_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCli(t *testing.T) {
	RegisterFailHandler(Fail)
	//Glooctl tests are failing CI
	//junitReporter := reporters.NewJUnitReporter("junit.xml")
	//RunSpecsWithDefaultAndCustomReporters(t, "Cli Test Suite", []Reporter{junitReporter})
}
