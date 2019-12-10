package checks

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestChecks(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Checks Suite")
}
