package probes_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestProbes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Probes Suite")
}
