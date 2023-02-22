package statusutils_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestStatusUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Status Utils Suite")
}
