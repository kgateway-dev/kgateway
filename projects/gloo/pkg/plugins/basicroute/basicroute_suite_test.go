package basicroute_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestBasicRoute(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BasicRoute Suite")
}
