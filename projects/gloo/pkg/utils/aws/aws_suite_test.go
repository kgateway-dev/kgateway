package aws_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAws(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpec(t, "Aws Suite")
}
