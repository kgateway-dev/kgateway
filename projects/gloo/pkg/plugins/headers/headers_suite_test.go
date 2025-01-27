package headers_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHeaders(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Headers Suite")
}
