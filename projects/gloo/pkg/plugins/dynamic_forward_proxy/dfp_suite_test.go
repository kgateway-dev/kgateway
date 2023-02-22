package dynamic_forward_proxy_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDfp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DynamicForwardProxy Suite")
}
