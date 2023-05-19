package perf_test

import (
	"testing"

	"github.com/solo-io/gloo/test/ginkgo/labels"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPerformance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Envoy Translator Syncer Performance Suite", Label(labels.Nightly, labels.Performance))
}
