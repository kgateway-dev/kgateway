package spec

import (
	"context"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
)

// In our Kubernetes E2E tests, we rely on a kubectl.Cli as the implementation for this interface
// As a result, we add a compile-time assertion to require that this does not drift
var _ manifestReaderWriter = new(kubectl.Cli)

// manifestReaderWriter is the minimal interface that a SpecRunner relies upon
type manifestReaderWriter interface {
	ApplyFile(ctx context.Context, fileName string, extraArgs ...string) error
	DeleteFile(ctx context.Context, fileName string, extraArgs ...string) error
}
