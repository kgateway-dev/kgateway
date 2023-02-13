package syncer

import (
	"testing"

	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/test/helpers"
)

func TestSyncer(t *testing.T) {
	RegisterFailHandler(Fail)
	helpers.SetupLog()
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Syncer Suite", []Reporter{junitReporter})
}
