package helpers

import (
	"testing"

	"go.uber.org/goleak"
)

func DeferredGoroutineLeakDetector(t *testing.T) func() {
	leakOptions := []goleak.Option{
		goleak.IgnoreCurrent(),
		goleak.IgnoreTopFunction("github.com/onsi/ginkgo/internal/specrunner.(*SpecRunner).registerForInterrupts"),
	}

	return func() {
		goleak.VerifyNone(t, leakOptions...)
	}
}
