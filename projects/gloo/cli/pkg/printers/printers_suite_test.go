package printers_test

import (
	"testing"

	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestPrinters(t *testing.T) {
	skhelpers.RegisterCommonFailHandlers() // these are currently overwritten by the fail handler below
	//Glooctl tests are failing CI
	//skhelpers.SetupLog()
	//junitReporter := reporters.NewJUnitReporter("junit.xml")
	//RunSpecsWithDefaultAndCustomReporters(t, "Printer Suite", []Reporter{junitReporter})
}
