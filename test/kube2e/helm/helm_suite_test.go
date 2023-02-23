package helm_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/go-utils/log"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestHelm(t *testing.T) {
	if os.Getenv("KUBE2E_TESTS") != "helm" {
		log.Warnf("This test is disabled. To enable, set KUBE2E_TESTS to 'helm' in your env.")
		return
	}
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Helm Suite")
}
