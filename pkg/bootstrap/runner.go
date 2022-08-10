package bootstrap

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
)

// Runner defines the behavior that will execute whenever Settings are updated
type Runner interface {
	// Run is the entrypoint for all runners. It is provided the following parameters:
	//	Context - The context used for the current run
	//	SharedCache - The kube cache used to register clients and receive notifications on changes to resources
	//	InMemoryResourceCache - The in memory cache used to persist local resources
	//	Settings - The current state of the Settings CR for the current run
	Run(ctx context.Context, kubeCache kube.SharedCache, inMemoryCache memory.InMemoryResourceCache, settings *v1.Settings) error
}
