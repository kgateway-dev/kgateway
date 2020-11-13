package sanitize_cluster_header_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestExtAuth(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SanitizeClusterHeader Suite")
}
