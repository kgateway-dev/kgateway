package syncer

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
)

// TODO: This is a temporary fix to avoid starting the CPU-intensive watch on kube config maps
func NewNoWatchArtifactClient(rcFactory factory.ResourceClientFactory) (v1.ArtifactClient, error) {
	client, err := v1.NewArtifactClient(rcFactory)
	if err != nil {
		return nil, err
	}
	return &noWatchArtifactClient{client}, nil
}

type noWatchArtifactClient struct {
	v1.ArtifactClient
}

func (noWatchArtifactClient) Watch(_ string, _ clients.WatchOpts) (<-chan v1.ArtifactList, <-chan error, error) {
	return nil, nil, nil
}
