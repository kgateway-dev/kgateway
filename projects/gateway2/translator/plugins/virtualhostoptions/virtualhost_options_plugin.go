package virtualhostoptions

import (
	"context"

	"github.com/rotisserie/eris"
	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	vhoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/virtualhostoptions/query"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
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

	// attachedOptions represents all VirtualHostOptions targeting the Gateway on which this listener resides
	attachedOptions := getAttachedVirtualHostOptions(ctx, listenerCtx.Gateway, p.vhOptQueries)
	if len(attachedOptions) == 0 {
		return nil
	}

	optsWithSectionName := map[string]*solokubev1.VirtualHostOption{}
	optsWithoutSectionName := []*solokubev1.VirtualHostOption{}
	for _, opt := range attachedOptions {
		if sectionName := opt.Spec.GetTargetRef().GetSectionName(); sectionName != nil && sectionName.GetValue() != "" {
			optsWithSectionName[sectionName.GetValue()] = opt
		} else {
			optsWithoutSectionName = append(optsWithoutSectionName, opt)
		}
	}

	if len(optsWithoutSectionName) > 1 {
		return eris.Errorf("expected 1 VirtualHostOption resource targeting listener %s's Gateway; got %d", listenerCtx.GwListener.Name, len(optsWithoutSectionName))
	}

	var optToUse *solokubev1.VirtualHostOption
	// If there is not a section name or the specified section name matches our listener, apply the vhost options
	if targetedOpt, ok := optsWithSectionName[string(listenerCtx.GwListener.Name)]; ok {
		optToUse = targetedOpt
	} else if len(optsWithoutSectionName) == 1 {
		optToUse = optsWithoutSectionName[0]
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
		vh.Options = optToUse.Spec.GetOptions()
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

func getAttachedVirtualHostOptions(ctx context.Context, gw *gwv1.Gateway, queries vhoptquery.VirtualHostOptionQueries) []*solokubev1.VirtualHostOption {
	var vhOptionList solokubev1.VirtualHostOptionList
	err := queries.GetVirtualHostOptionsForGateway(ctx, gw, &vhOptionList)
	if err != nil {
		contextutils.LoggerFrom(ctx).Errorf("error while Listing VirtualHostOptions: %v", err)
		// TODO: add status to policy on error
		return nil
	}

	// as the VirtualHostOptionList does not contain pointers, and VirtualHostOption is a concrete proto message,
	// we need to turn it into a pointer slice to avoid copying proto message state around, copying locks, etc.
	// while we perform operations on the VirtualHostOptionList
	ptrSlice := []*solokubev1.VirtualHostOption{}
	items := vhOptionList.Items
	for i := range items {
		ptrSlice = append(ptrSlice, &items[i])
	}
	return ptrSlice
}
