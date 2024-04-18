package als_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAls(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Als Suite")
	// Not a spec but runnable without ginko. Make sure that it runs in CI by adding here!
	TestDetectUnusefulCmds(t)
}
