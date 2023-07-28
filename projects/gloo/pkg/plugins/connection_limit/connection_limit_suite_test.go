package connection_limit

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConnectionLimit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Connection Limit Suite")
}
