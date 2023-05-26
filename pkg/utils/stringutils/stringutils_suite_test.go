package stringutils_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestStringUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "StringUtils Suite")
}
