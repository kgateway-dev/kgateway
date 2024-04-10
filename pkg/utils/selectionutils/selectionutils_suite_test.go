package selectionutils_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSelectionUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Selection utils Suite")
}
