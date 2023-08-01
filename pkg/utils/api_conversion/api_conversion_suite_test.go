package api_conversion_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestChannelutils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API Conversion Suite")
}
