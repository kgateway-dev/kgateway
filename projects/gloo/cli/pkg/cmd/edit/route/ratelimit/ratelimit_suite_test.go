package ratelimit_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRateLimit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RateLimit Suite")
}
