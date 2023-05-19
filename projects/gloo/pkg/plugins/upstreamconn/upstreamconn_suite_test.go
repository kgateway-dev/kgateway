package upstreamconn_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUpstreamconn(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Upstreamconn Suite")
}
