package tunneling_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTunneling(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTP Tunneling Suite")
}
