package headermodifier_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHeadermodifier(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Headermodifier Suite")
}
