package localgloo_e2e_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestLocalglooE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "LocalglooE2e Suite")
}
