package ratelimit_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestRatelimit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ratelimit Suite")
}
