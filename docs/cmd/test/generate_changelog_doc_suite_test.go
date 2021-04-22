package main_test

import (
	"testing"

	"github.com/onsi/ginkgo/reporters"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/go-utils/testutils"
)

func TestGenerateChangelogDoc(t *testing.T) {
	RegisterFailHandler(Fail)
	testutils.RegisterCommonFailHandlers()
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Generate Changelog Suite", []Reporter{junitReporter})
}
