package printers_test

import (
	"testing"

	. "github.com/onsi/ginkgo"

	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestPrinters(t *testing.T) {
	skhelpers.RegisterCommonFailHandlers() // these are currently overwritten by the fail handler below
	skhelpers.SetupLog()
	RunSpecs(t, "Printer Suite")
}
