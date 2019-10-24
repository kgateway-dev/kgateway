package sanitizer_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSanitizer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sanitizer Suite")
}
