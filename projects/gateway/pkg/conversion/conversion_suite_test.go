package conversion_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestConversion(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Conversion Suite")
}
