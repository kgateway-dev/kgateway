package loadbalancer_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLoadbalancer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Loadbalancer Suite")
}
