package httproute

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRoute(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Route Suite")
}
