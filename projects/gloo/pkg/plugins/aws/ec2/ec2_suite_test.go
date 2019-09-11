package ec2

import (
	"testing"

	"github.com/solo-io/go-utils/testutils"

	. "github.com/onsi/ginkgo"
)

func TestEc2(t *testing.T) {
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "EC2 Suite")
}
