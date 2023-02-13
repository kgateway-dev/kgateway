package faultinjection_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFaultInjection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fault Injection Suite")
}
