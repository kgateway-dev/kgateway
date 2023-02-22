package cli_unit_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCli(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cli Test Suite")
}
