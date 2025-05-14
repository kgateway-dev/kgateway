package downward_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDownward(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Downward Suite")
}
