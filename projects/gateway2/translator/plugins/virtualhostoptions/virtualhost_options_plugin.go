package virtualhostoptions

import (
	"context"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	vhoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/virtualhostoptions/query"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ plugins.ListenerPlugin = &plugin{}

var routeOptionGK = schema.GroupKind{
	Group: sologatewayv1.RouteOptionGVK.Group,
	Kind:  sologatewayv1.RouteOptionGVK.Kind,
}

type plugin struct {
	gwQueries    gwquery.GatewayQueries
	vhOptQueries vhoptquery.VirtualHostOptionQueries
}

func NewPlugin(gwQueries gwquery.GatewayQueries, client client.Client) *plugin {
	return &plugin{
		gwQueries,
		vhoptquery.NewQuery(client),
	}
}

func (p *plugin) ApplyListenerPlugin(
	ctx context.Context,
	listenerCtx *plugins.ListenerContext,
	outListener *v1.Listener,
) error {

	// attachedOption represents the VirtualHostOptions targeting the Gateway on which this listener resides, and/or
	// the VirtualHostOptions which specifies this listener in section name
	attachedOption, err := p.vhOptQueries.GetVirtualHostOptionsForListener(ctx, listenerCtx.GwListener, listenerCtx.Gateway)
	if err != nil {
		return err
	}

	if attachedOption == nil {
		return nil
	}

	var vhs []*v1.VirtualHost
	switch outListener.GetListenerType().(type) {
	case *v1.Listener_HttpListener:
		vhs = outListener.GetHttpListener().GetVirtualHosts()
	case *v1.Listener_HybridListener:
		matchedListeners := outListener.GetHybridListener().GetMatchedListeners()
		for _, ml := range matchedListeners {
			if httpListener := ml.GetHttpListener(); httpListener != nil {
				vhs = append(vhs, httpListener.GetVirtualHosts()...)
			}
		}

	case *v1.Listener_AggregateListener:
		for _, v := range outListener.GetAggregateListener().GetHttpResources().GetVirtualHosts() {
			v := v
			vhs = append(vhs, v)
		}
	default:
		// no http on listener
		return nil
	}

	for _, vh := range vhs {
		vh.Options = attachedOption.Spec.GetOptions()
	}

	return nil
}

func (p *plugin) handleAttachment(
	ctx context.Context,
	listenerCtx *plugins.ListenerContext,
	outListener *v1.Listener,
) {

	return
}
