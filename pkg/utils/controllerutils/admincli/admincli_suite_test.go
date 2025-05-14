package admincli_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAdminCli(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AdminCli Suite")
}
