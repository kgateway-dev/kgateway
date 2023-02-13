package consul

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var T *testing.T

func TestConsul(t *testing.T) {
	RegisterFailHandler(Fail)
	T = t
	RunSpecs(t, "Consul Upstream Client Suite")
}
