package http_path_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestHttpPath(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HttpPath Suite")
}
