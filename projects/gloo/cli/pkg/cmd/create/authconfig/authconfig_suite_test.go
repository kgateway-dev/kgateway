package authconfig_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAuthConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthConfig Suite")
}
