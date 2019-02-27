package knative_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestKnative(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Knative Suite")
}
