package syncer

import (
	"context"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
)

func Setup(ctx context.Context, kubeCache kube.SharedCache, inMemoryCache memory.InMemoryResourceCache, settings *gloov1.Settings) error {
	//TODO (after gateway removal feature branch merges)
	//This file and any places where this setup was called can be removed
	//I am removing the code from this file as part of the feature branch because a lot of it is duplicated in gloo and the diff was getting confusing
	// WRT what changes were relevant and which were effectively dead code.
	return nil
}
