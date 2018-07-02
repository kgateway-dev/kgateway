package fission_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestFission(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fission Suite")
}
