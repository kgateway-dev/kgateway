package timeout_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestTimeout(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Timeout Suite")
}
