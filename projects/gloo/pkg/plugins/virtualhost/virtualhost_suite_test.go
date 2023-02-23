package virtualhost_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVirtualHost(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Virtual Host Suite")
}
