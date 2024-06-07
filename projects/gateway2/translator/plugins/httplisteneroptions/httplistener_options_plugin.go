package httplisteneroptions

import (
	"context"
	"strconv"

	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	httplisquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/httplisteneroptions/query"
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
	// attachedOption represents the ListenerOptions targeting the Gateway on which this listener resides, and/or
	// the ListenerOptions which specifies this listener in section name
	attachedOptions, err := p.httpLisOptQueries.GetAttachedHttpListenerOptions(ctx, listenerCtx.GwListener, listenerCtx.Gateway)
	if err != nil {
		return err
	}

	if len(attachedOptions) == 0 {
		return nil
	}

	optToUse := attachedOptions[0]

	if optToUse == nil {
		// unsure if this should be an error case
		return nil
	}

	// Currently we only create AggregateListeners in k8s gateway translation.
	// If that ever changes, we will need to handle other listener types more gracefully here.
	aggListener := outListener.GetAggregateListener()
	if aggListener == nil {
		return nil //TODO
	}

	httpOptions := optToUse.Spec.GetOptions()

	// store HttpListenerOptions, indexed by a hash of the httpOptions
	httpOptionsByName := map[string]*gloov1.HttpListenerOptions{}
	httpOptionsHash, _ := httpOptions.Hash(nil)
	httpOptionsRef := strconv.Itoa(int(httpOptionsHash))
	httpOptionsByName[httpOptionsRef] = httpOptions

	aggListener.GetHttpResources().HttpOptions = httpOptionsByName

	// set the ref on each HttpFilterChain in this listener
	for _, hfc := range aggListener.GetHttpFilterChains() {
		hfc.HttpOptionsRef = httpOptionsRef
	}

	return nil
}
