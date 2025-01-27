package transforms_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/solo-kit/test/helpers"
)

func TestTransforms(t *testing.T) {
	helpers.RegisterCommonFailHandlers()
	helpers.SetupLog()

	RunSpecs(t, "Transforms Suite")
}
