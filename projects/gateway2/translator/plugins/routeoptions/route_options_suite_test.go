package routeoptions

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRouteOptionsPlugin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RouteOptions Plugin Suite")
}
