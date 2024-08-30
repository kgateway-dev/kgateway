package sliceutils_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSliceUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SliceUtils Suite")
}
