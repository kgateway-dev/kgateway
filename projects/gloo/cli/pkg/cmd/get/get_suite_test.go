package get_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGet(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Get Suite")
}
