package extauth_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestExtauth(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Extauth Suite")
}
