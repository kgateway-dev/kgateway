package als_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAls(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Als Suite")
}
