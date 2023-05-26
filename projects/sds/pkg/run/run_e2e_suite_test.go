package run_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSDS_E2E_Server(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SDS Server E2E Test Suite")
}
