package tcp_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestTcp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "tcp Suite")
}
