package reporting_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestReporting(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Reporting Suite")
}
