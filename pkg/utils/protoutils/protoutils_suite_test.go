package protoutils_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestProtoutils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Protoutils Suite")
}
