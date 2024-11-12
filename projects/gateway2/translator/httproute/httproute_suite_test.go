package httproute_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHttproute(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Httproute Suite")
}
