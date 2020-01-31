package gzip_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestGzip(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gzip Suite")
}
