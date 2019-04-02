package printers_test

import (
	"testing"

	. "github.com/onsi/ginkgo"

	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestPrinters(t *testing.T) {
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Printer Suite")
}
