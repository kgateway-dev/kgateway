package ec2

import (
	"testing"

	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/go-utils/testutils"
)

func TestEc2(t *testing.T) {
	testutils.RegisterCommonFailHandlers()
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "EC2 Suite", []Reporter{junitReporter})
}
