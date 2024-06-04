package listeneroptions

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestListenerOptionsPlugin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ListenerOptions Plugin Suite")
}
