package testutils

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKube2eTestUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kube2e TestUtils Suite")
}
