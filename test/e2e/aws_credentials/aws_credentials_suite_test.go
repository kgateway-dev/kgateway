package aws_credentials

import (
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/solo-io/solo-kit/test/helpers"
)

// These tests are isolated from the other e2e tests because they do not need envoy to be running
func TestAwsCredentials(t *testing.T) {
	helpers.RegisterCommonFailHandlers()
	RunSpecs(t, "AWS Credentials Suite")
}
