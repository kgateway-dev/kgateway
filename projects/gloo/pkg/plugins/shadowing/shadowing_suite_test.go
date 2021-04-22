package shadowing_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	"github.com/solo-io/go-utils/testutils"
)

func TestTracing(t *testing.T) {
	testutils.RegisterCommonFailHandlers()
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Shadowing Suite", []Reporter{junitReporter})
}
