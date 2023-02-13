package plugins

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPluginInterface(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Plugin Interface Suite")
}
