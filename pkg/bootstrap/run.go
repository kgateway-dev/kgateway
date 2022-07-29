package bootstrap

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
)

// RunnerFactory is executed each time Settings are changed
// It returns a Runnable function, according to the Settings, or an error if a Runner could not be generated
type RunnerFactory func(
	ctx context.Context,
	kubeCache kube.SharedCache,
	inMemoryCache memory.InMemoryResourceCache,
	settings *v1.Settings,
) (RunFunc, error)

// RunFunc is executed each time Settings are changed
type RunFunc func() error
