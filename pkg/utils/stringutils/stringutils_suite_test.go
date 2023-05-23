package stringutils_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestStringUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Regex Suite")
}
