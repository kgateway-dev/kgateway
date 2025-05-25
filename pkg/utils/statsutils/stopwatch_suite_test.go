package statsutils_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestStopWatchSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "StopWatch Suite")
}
