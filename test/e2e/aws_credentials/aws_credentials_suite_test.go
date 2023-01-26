package aws_credentials

import (
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	"github.com/solo-io/solo-kit/test/helpers"
)

// These tests are isolated from the other e2e tests because they do not need envoy to be running
const (
	region        = "us-east-1"
	roleArnEnvVar = "AWS_ROLE_ARN"
)

func TestAwsCredentials(t *testing.T) {
	helpers.RegisterCommonFailHandlers()
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "AWS Credentials Suite", []Reporter{junitReporter})
}
