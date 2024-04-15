package virtualhostoptions

import (
	"context"

	"github.com/rotisserie/eris"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	vhoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/virtualhostoptions/query"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ plugins.ListenerPlugin = &plugin{}

type plugin struct {
	gwQueries    gwquery.GatewayQueries
	vhOptQueries vhoptquery.VirtualHostOptionQueries
}

var (
	ErrUnexpectedListenerType = eris.New("unexpected listener type")
	errUnexpectedListenerType = func(l *v1.Listener) error {
		return eris.Wrapf(ErrUnexpectedListenerType, "expected AggregateListener, got %T", l.GetListenerType())
	}
)

func NewPlugin(gwQueries gwquery.GatewayQueries, client client.Client) *plugin {
	return &plugin{
		gwQueries:    gwQueries,
		vhOptQueries: vhoptquery.NewQuery(client),
	}
}

func (p *plugin) ApplyListenerPlugin(
	ctx context.Context,
	listenerCtx *plugins.ListenerContext,
	outListener *v1.Listener,
) error {
	// Currently we only create AggregateListeners in k8s gateway translation.
	// If that ever changes, we will need to handle other listener types more gracefully here.
	aggListener := outListener.GetAggregateListener()
	if aggListener == nil {
		return errUnexpectedListenerType(outListener)
	}

	// attachedOption represents the VirtualHostOptions targeting the Gateway on which this listener resides, and/or
	// the VirtualHostOptions which specifies this listener in section name
	attachedOptions, err := p.vhOptQueries.GetVirtualHostOptionsForListener(ctx, listenerCtx.GwListener, listenerCtx.Gateway)
	if err != nil {
		return err
	}

	if attachedOptions == nil || len(attachedOptions) == 0 {
		return nil
	}

	if numOpts := len(attachedOptions); numOpts > 1 {
		// TODO: Report conflicts on the [1:] options
	}

	if attachedOptions[0] == nil {
		// unsure if this should be an error case
		return nil
	}

	for _, v := range aggListener.GetHttpResources().GetVirtualHosts() {
		v.Options = attachedOptions[0].Spec.GetOptions()
	}

	return nil
}
