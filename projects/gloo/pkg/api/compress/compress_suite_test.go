package compress_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCompress(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Compress Suite")
}
