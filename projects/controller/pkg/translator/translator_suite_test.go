package translator_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var T *testing.T

func TestTranslator(t *testing.T) {
	RegisterFailHandler(Fail)
	T = t
	RunSpecs(t, "Translator Suite")
}
