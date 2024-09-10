package directresponse_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDirectResponse(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DirectResponse Suite")
}
