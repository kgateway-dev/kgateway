package listeneroptions

import (
	"context"

	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	lisquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/listeneroptions/query"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ plugins.ListenerPlugin = &plugin{}

type plugin struct {
	gwQueries     gwquery.GatewayQueries
	lisOptQueries lisquery.ListenerOptionQueries
}

func NewPlugin(
	gwQueries gwquery.GatewayQueries,
	client client.Client,
) *plugin {
	return &plugin{
		gwQueries:     gwQueries,
		lisOptQueries: lisquery.NewQuery(client),
	}
}

func (p *plugin) ApplyListenerPlugin(
	ctx context.Context,
	listenerCtx *plugins.ListenerContext,
	outListener *v1.Listener,
) error {
	// attachedOption represents the ListenerOptions targeting the Gateway on which this listener resides, and/or
	// the ListenerOptions which specifies this listener in section name
	attachedOptions, err := p.lisOptQueries.GetAttachedListenerOptions(ctx, listenerCtx.GwListener, listenerCtx.Gateway)
	if err != nil {
		return err
	}

	if len(attachedOptions) == 0 {
		return nil
	}

	// use the first option (highest in priority)
	// see for more context: https://github.com/solo-io/solo-projects/issues/6313
	optToUse := attachedOptions[0]
	outListener.Options = optToUse.Spec.GetOptions()

	return nil
}
