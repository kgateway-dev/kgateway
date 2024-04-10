package errutils

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestErrUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ErrUtils Suite")
}
