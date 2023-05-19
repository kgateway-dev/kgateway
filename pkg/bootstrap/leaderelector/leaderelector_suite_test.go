package leaderelector_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

func TestLeaderElector(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Leader Elector Suite")
}
