package local_ratelimit

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLocalRateLimit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Local Rate Limit Suite")
}
