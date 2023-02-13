package remove_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRemove(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Remove Suite")
}
