package authconfig_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAuthConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthConfig Suite")
}
