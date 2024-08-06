package metrics

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	. "github.com/onsi/gomega"
)

func TestUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metric Utils Suite")
}
