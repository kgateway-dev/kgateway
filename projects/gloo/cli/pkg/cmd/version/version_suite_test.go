package version_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
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
