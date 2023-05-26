package grpc

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/test/gomega"
	"github.com/solo-io/go-utils/log"
)

func TestGrpc(t *testing.T) {
	SetAsyncAssertionDefaults(AsyncAssertionDefaults{})
	RegisterFailHandler(Fail)

	log.DefaultOut = GinkgoWriter
	RunSpecs(t, "Grpc Suite")
}
