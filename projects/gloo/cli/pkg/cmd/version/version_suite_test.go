package version

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	T *testing.T
)

func TestVersion(t *testing.T) {
	T = t
	RegisterFailHandler(Fail)
	RunSpecs(t, "Version Suite")
}
