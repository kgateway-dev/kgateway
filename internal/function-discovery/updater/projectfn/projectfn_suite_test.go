package fission_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestProjectfn(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Projectfn Suite")
}
