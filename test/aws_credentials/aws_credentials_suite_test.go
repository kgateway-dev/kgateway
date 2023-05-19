package aws_credentials

import (
	"testing"

	testhelpers "github.com/solo-io/gloo/test/testutils"

	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/solo-kit/test/helpers"
)

const (
	region        = "us-east-1"
	roleArnEnvVar = "AWS_ROLE_ARN"
)

func TestAwsCredentials(t *testing.T) {
	helpers.RegisterCommonFailHandlers()
	RunSpecs(t, "AWS Credentials Suite")
}

var _ = BeforeSuite(func() {
	testhelpers.ValidateRequirementsAndNotifyGinkgo(testhelpers.AwsCredentials())
})
