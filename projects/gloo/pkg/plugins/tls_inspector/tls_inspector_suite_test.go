package tls_inspector_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestTlsInspector(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TLS Inspector Suite")
}
