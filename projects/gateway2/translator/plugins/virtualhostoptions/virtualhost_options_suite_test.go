package virtualhostoptions

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVirtualHostOptionsPlugin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VirtualHostOptions Plugin Suite")
}
