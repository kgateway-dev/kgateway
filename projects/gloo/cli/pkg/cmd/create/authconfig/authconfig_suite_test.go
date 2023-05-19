package authconfig_test

import (
	"testing"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAuthConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthConfig Suite")
}

var _ = BeforeSuite(func() {
	helpers.UseMemoryClients()
})
