package cliutil_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCliutil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cliutil Suite")
}
