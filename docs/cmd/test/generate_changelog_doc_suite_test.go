package main_test

import (
	"github.com/solo-io/go-utils/testutils"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestGenerateChangelogDoc(t *testing.T) {
	RegisterFailHandler(Fail)
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Generate Changelog Suite")
}
