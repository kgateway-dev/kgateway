package tunneling_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestTunneling(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTP Tunneling Suite")
}
