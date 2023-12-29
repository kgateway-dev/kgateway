package urlrewrite_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUrlRewrite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UrlRewrite Suite")
}
