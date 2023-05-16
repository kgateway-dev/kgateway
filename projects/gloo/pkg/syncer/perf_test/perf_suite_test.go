package perf_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/gomega/labels"
	"testing"
)

func TestPerformance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Envoy Translator Syncer Performance Suite", Label(labels.Nightly, labels.Performance))
}
