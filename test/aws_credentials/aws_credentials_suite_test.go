package aws_credentials

import (
	"testing"

	testhelpers "github.com/solo-io/gloo/test/helpers"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	"github.com/solo-io/solo-kit/test/helpers"
)

const (
	region        = "us-east-1"
	roleArnEnvVar = "AWS_ROLE_ARN"
)

func TestAwsCredentials(t *testing.T) {
	helpers.RegisterCommonFailHandlers()
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "AWS Credentials Suite", []Reporter{junitReporter})
}

var _ = BeforeSuite(func() {
	testhelpers.ValidateRequirementsAndNotifyGinkgo(testhelpers.AwsCredentials())
})
