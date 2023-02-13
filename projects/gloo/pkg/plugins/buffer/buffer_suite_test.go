package buffer_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBuffer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Buffer Suite")
}
