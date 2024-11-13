package iosnapshot

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIoSnapshot(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "IOSnapshot Suite")
}
