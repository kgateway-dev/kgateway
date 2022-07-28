package runner_test

import (
    "testing"

    . "github.com/onsi/ginkgo"
    "github.com/onsi/ginkgo/reporters"
    . "github.com/onsi/gomega"
)

func TestSyncer(t *testing.T) {
    RegisterFailHandler(Fail)
    junitReporter := reporters.NewJUnitReporter("junit.xml")
    RunSpecsWithDefaultAndCustomReporters(t, "Runner Suite", []Reporter{junitReporter})
}
