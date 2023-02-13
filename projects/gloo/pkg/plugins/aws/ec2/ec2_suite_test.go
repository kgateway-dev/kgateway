package ec2

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/go-utils/testutils"
)

func TestEc2(t *testing.T) {
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "EC2 Suite")
}
