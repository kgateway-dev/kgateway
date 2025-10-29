package assertions_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAssertions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Assertions Suite")
}
