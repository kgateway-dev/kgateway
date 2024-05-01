package validation

import (
	"context"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// contains data needed to bootstrap a GW2 translator
type ValidationHelper struct {
	K8sGwExtensions extensions.K8sGatewayExtensions
	GatewayQueries  query.GatewayQueries
	Cl              client.Client
}

type ValidatorClient struct {
}

func (v *ValidatorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}

func (v *ValidationHelper) TranslateK8sGatewayProxies(ctx context.Context, res resources.Resource) ([]*gloov1.Proxy, error) {
	// we need to find the Gateway associated with the resource
	rtOpt, ok := res.(*sologatewayv1.RouteOption)
	if !ok {
		panic("uh oh")
	}

	// first find the target HTTPRoute
	var hr gwv1.HTTPRoute
	targetRef := rtOpt.GetTargetRef()
	hrnn := types.NamespacedName{
		Namespace: targetRef.GetNamespace().GetValue(),
		Name:      targetRef.GetName(),
	}
	err := v.Cl.Get(ctx, hrnn, &hr, &client.GetOptions{})
	if err != nil {
		panic("err getting")
	}

	// now we directly get the parentRefs (which are Gateways!)
	var gws []gwv1.Gateway
	for _, pr := range hr.Spec.ParentRefs {
		var gw gwv1.Gateway
		gwnn := types.NamespacedName{
			Namespace: string(*pr.Namespace),
			Name:      string(pr.Name),
		}
		err := v.Cl.Get(ctx, gwnn, &gw, &client.GetOptions{})
		if err != nil {
			panic("err getting GW")
		}
		gws = append(gws, gw)
	}

	// create the plugins and translator
	plugins := v.K8sGwExtensions.CreatePluginRegistry(ctx)
	t := translator.NewTranslator(v.GatewayQueries, plugins)
	rm := reports.NewReportMap()
	r := reports.NewReporter(&rm)

	// translate all the gateways and collect the output Proxies
	var proxies []*gloov1.Proxy
	for _, gw := range gws {
		proxy := t.TranslateProxy(ctx, &gw, "gloo-system", r)
		if proxy == nil || len(proxy.GetListeners()) == 0 {
			continue
		}
		proxies = append(proxies, proxy)
	}

	// we currently don't want to fail validation for any K8s Gateway-specific error conditions, as the behavior and status reporting
	// requirements for these scenarios are built into the spec; let's always return a nil error until that decision changes
	return proxies, nil
}
