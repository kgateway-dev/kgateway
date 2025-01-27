package linkerd_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLinkerd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Linkerd Suite")
}
