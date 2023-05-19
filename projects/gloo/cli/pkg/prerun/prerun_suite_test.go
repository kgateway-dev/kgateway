package prerun_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	T *testing.T
)

func TestPrerun(t *testing.T) {
	T = t
	RegisterFailHandler(Fail)
	RunSpecs(t, "PreRun Suite")
}
