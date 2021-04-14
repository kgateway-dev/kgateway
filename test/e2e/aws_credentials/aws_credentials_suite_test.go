package aws_credentials

import (
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	"github.com/solo-io/solo-kit/test/helpers"
)

// These tests are isolated from the other e2e tests because they do not need envoy to be running
func TestAwsCredentials(t *testing.T) {
	helpers.RegisterCommonFailHandlers()
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "AWS Credentials Suite", []Reporter{junitReporter})
}
