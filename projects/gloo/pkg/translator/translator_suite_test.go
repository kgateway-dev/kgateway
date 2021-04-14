package translator_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
)

var T *testing.T

func TestTranslator(t *testing.T) {
	RegisterFailHandler(Fail)
	T = t
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Translator Suite", []Reporter{junitReporter})
}
