package hcm_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHcm(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hcm Suite")
}
