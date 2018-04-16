package cloudfoundry

import (
	"context"

	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/plugins"
)

func init() {
	plugins.Register(nil, createEndpointDiscovery)
}

func createEndpointDiscovery(opts bootstrap.Options) (endpointdiscovery.Interface, error) {
	istioclient, err := GetClientFromOptions(opts.CoPilotOptions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create copilot client")
	}
	resyncDuration := opts.ConfigStorageOptions.SyncFrequency
	disc := NewEndpointDiscovery(context.Background(), istioclient, resyncDuration)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start copilot endpoint discovery")
	}
	return disc, err
}
