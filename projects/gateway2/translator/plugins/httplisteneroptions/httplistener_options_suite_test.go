package httplisteneroptions

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHttpListenerOptionsPlugin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HttpListenerOptions Plugin Suite")
}
