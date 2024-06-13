package httplisteneroptions

import (
	"context"

	edgegwutils "github.com/solo-io/gloo/projects/gateway/pkg/translator/utils"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	httplisquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/httplisteneroptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ plugins.ListenerPlugin = &plugin{}

type plugin struct {
	gwQueries         gwquery.GatewayQueries
	httpLisOptQueries httplisquery.HttpListenerOptionQueries
}

func NewPlugin(
	gwQueries gwquery.GatewayQueries,
	client client.Client,
) *plugin {
	return &plugin{
		gwQueries:         gwQueries,
		httpLisOptQueries: httplisquery.NewQuery(client),
	}
}

func (p *plugin) ApplyListenerPlugin(
	ctx context.Context,
	listenerCtx *plugins.ListenerContext,
	outListener *gloov1.Listener,
) error {
	// attachedOption represents the HttpListenerOptions targeting the Gateway on which this listener resides, and/or
	// the HttpListenerOptions which specifies this listener in section name
	attachedOptions, err := p.httpLisOptQueries.GetAttachedHttpListenerOptions(ctx, listenerCtx.GwListener, listenerCtx.Gateway)
	if err != nil {
		return err
	}

	if len(attachedOptions) == 0 {
		return nil
	}

	// Currently we only create AggregateListeners in k8s gateway translation.
	// If that ever changes, we will need to handle other listener types more gracefully here.
	aggListener := outListener.GetAggregateListener()
	if aggListener == nil {
		return utils.ErrUnexpectedListener(outListener)
	}

	// use the first option (highest in priority)
	// see for more context: https://github.com/solo-io/solo-projects/issues/6313
	optToUse := attachedOptions[0]
	httpOptions := optToUse.Spec.GetOptions()

	// store HttpListenerOptions, indexed by a hash of the httpOptions
	httpOptionsByName := map[string]*gloov1.HttpListenerOptions{}
	httpOptionsRef := edgegwutils.HashAndStoreHttpOptions(httpOptions, httpOptionsByName)

	aggListener.GetHttpResources().HttpOptions = httpOptionsByName

	// set the ref on each HttpFilterChain in this listener
	for _, hfc := range aggListener.GetHttpFilterChains() {
		hfc.HttpOptionsRef = httpOptionsRef
	}

	return nil
}
