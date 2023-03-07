package azure_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAzure(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Azure Suite")
}
