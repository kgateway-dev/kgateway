package protocoloptions_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestProtocolOptions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Protocol Options Suite")
}
