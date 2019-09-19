package validation_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var T *testing.T

func TestValidation(t *testing.T) {
	T = t
	RegisterFailHandler(Fail)
	RunSpecs(t, "Validation Suite")
}
