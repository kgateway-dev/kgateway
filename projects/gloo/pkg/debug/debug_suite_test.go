package debug_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var T *testing.T

func TestDebugEndpoint(t *testing.T) {
	RegisterFailHandler(Fail)
	T = t
	RunSpecs(t, "Proxy Debug Suite")
}
