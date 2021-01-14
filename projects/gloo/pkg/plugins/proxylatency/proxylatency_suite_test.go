package proxylatency_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestProxylatency(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Proxylatency Suite")
}
