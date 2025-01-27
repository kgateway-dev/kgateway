package upgradeconfig_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUpgradeconfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Upgradeconfig Suite")
}
