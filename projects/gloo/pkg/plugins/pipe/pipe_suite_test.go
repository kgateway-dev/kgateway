package pipe_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestPipe(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pipe Suite")
}
