package virtualservice_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestVirtualService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VirtualService Suite")
}
