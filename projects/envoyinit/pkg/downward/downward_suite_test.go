package downward_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDownward(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Downward Suite")
}
