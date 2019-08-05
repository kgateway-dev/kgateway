package selection_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSelection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Selection Suite")
}
