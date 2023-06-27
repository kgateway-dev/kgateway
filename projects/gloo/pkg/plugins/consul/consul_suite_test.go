package consul

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConsul(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Consul Plugin Suite")
}
