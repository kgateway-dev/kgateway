package upstream_proxy_protocol

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUpstreamProxyProtocol(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UpstreamProxyProtocol Suite")
}
