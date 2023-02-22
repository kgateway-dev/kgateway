package kube_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKube(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kube Suite")
}
