package operations_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOperations(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Operations Suite")
}
