package authconfig_test

import (
	"testing"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/test/gomega"
)

func TestAuthConfig(t *testing.T) {
	SetAsyncAssertionDefaults(AsyncAssertionDefaults{})
	RegisterFailHandler(Fail)

	RunSpecs(t, "AuthConfig Suite")
}

var _ = BeforeSuite(func() {
	helpers.UseMemoryClients()
})
