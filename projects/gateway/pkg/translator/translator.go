package translator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/solo-io/go-utils/contextutils"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/reporter"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const GatewayProxyName = "gateway-proxy"

func Translate(ctx context.Context, namespace string, snap *v1.ApiSnapshot) (*gloov1.Proxy, reporter.ResourceErrors) {
	logger := contextutils.LoggerFrom(ctx)

	filteredGateways := filterGatewaysForNamespace(snap.Gateways, namespace)

	resourceErrs := make(reporter.ResourceErrors)
	resourceErrs.Accept(filteredGateways.AsInputResources()...)
	resourceErrs.Accept(snap.VirtualServices.AsInputResources()...)
	if len(filteredGateways) == 0 {
		logger.Debugf("%v had no gateways", snap.Hash())
		return nil, resourceErrs
	}
	if len(snap.VirtualServices) == 0 {
		logger.Debugf("%v had no virtual services", snap.Hash())
		return nil, resourceErrs
	}
	validateGateways(filteredGateways, resourceErrs)
	var listeners []*gloov1.Listener
	for _, gateway := range filteredGateways {
		virtualServices := getVirtualServiceForGateway(gateway, snap.VirtualServices, resourceErrs)
		filtered := filterVirtualServiceForGateway(gateway, virtualServices)
		mergedVirtualServices := validateAndMergeVirtualServices(namespace, gateway, filtered, resourceErrs)
		listener := desiredListener(gateway, mergedVirtualServices)
		listeners = append(listeners, listener)
	}
	return &gloov1.Proxy{
		Metadata: core.Metadata{
			Name:      GatewayProxyName,
			Namespace: namespace,
		},
		Listeners: listeners,
	}, resourceErrs
}

// https://github.com/solo-io/gloo/issues/538
// Gloo should only pay attention to gateways it creates, i.e. in it's write namespace, to support
// handling multiple gloo installations
func filterGatewaysForNamespace(gateways v1.GatewayList, namespace string) v1.GatewayList {
	var filteredGateways v1.GatewayList
	for _, gateway := range gateways {
		if gateway.Metadata.Namespace == namespace {
			filteredGateways = append(filteredGateways, gateway)
		}
	}
	return filteredGateways
}

func joinGatewayNames(gateways v1.GatewayList) string {
	var names []string
	for _, gw := range gateways {
		names = append(names, gw.Metadata.Name)
	}
	return strings.Join(names, ".")
}

func validateGateways(gateways v1.GatewayList, resourceErrs reporter.ResourceErrors) {
	bindAddresses := map[string]v1.GatewayList{}
	// if two gateway (=listener) that belong to the same proxy share the same bind address,
	// they are invalid.
	for _, gw := range gateways {
		bindAddress := fmt.Sprintf("%s:%d", gw.BindAddress, gw.BindPort)
		bindAddresses[bindAddress] = append(bindAddresses[bindAddress], gw)
	}

	for addr, gateways := range bindAddresses {
		if len(gateways) > 1 {
			for _, gw := range gateways {
				resourceErrs.AddError(gw, fmt.Errorf("bind-address %s is not unique in a proxy. gateways: %s", addr, strings.Join(gatewaysRefsToString(gateways), ",")))
			}
		}
	}
}

func gatewaysRefsToString(gateways v1.GatewayList) []string {
	var ret []string
	for _, gw := range gateways {
		ret = append(ret, gw.Metadata.Ref().Key())
	}
	return ret
}

func domainsToKey(domains []string) string {
	// copy before mutating for good measure
	domains = append([]string{}, domains...)
	// sort, and join all domains with an out of band character, like ','
	sort.Strings(domains)
	return strings.Join(domains, ",")
}

func validateAndMergeVirtualServices(ns string, gateway *v1.Gateway, virtualServices v1.VirtualServiceList, resourceErrs reporter.ResourceErrors) v1.VirtualServiceList {

	domainKeysSets := map[string]v1.VirtualServiceList{}
	for _, vs := range virtualServices {
		if vs.VirtualHost == nil {
			continue
		}
		domainsKey := domainsToKey(vs.VirtualHost.Domains)
		domainKeysSets[domainsKey] = append(domainKeysSets[domainsKey], vs)
	}

	domainSet := map[string][]string{}
	// make sure each domain is only in one domain set
	for k, vslist := range domainKeysSets {
		// take the first one as they are all the same
		domains := vslist[0].VirtualHost.Domains
		for _, d := range domains {
			domainSet[d] = append(domainSet[d], k)
		}
	}

	// report errors
	for domain, domainSetKeys := range domainSet {
		if len(domainSetKeys) > 1 {
			resourceErrs.AddError(gateway, fmt.Errorf("domain %s is present in more than one vservice set in this gateway", domain))
		}
	}
	// return merged list
	var mergedVirtualServices v1.VirtualServiceList
	for k, vslist := range domainKeysSets {
		if len(vslist) == 1 {
			// only one vservice, do nothing.
			mergedVirtualServices = append(mergedVirtualServices, vslist[0])
			continue
		}

		// take the first one as they are all the same
		var routes []*gloov1.Route
		var sslConfig *gloov1.SslConfig
		var vhostPlugins *gloov1.VirtualHostPlugins
		for _, vs := range vslist {
			routes = append(routes, vs.VirtualHost.Routes...)
			if sslConfig == nil {
				sslConfig = vs.SslConfig
			} else if vs.SslConfig != nil {
				resourceErrs.AddError(gateway, fmt.Errorf("more than one ssl config is present in virtual service of these domains: %s", k))
			}

			havePlugins := vs.VirtualHost != nil &&
				vs.VirtualHost.VirtualHostPlugins != nil

			if vhostPlugins == nil {
				if havePlugins {
					vhostPlugins = vs.VirtualHost.VirtualHostPlugins
				}
			} else if havePlugins {
				resourceErrs.AddError(gateway, fmt.Errorf("more than one vhost plugin is present in virtual service of these domains: %s", k))
			}
		}
		glooutils.SortRoutesByPath(routes)

		ref := core.Metadata{
			// name shouldnt matter as it this object is ephemeral.
			Name:      getMergedName(k),
			Namespace: ns,
		}
		mergedVs := &v1.VirtualService{
			VirtualHost: &gloov1.VirtualHost{
				Domains:            vslist[0].VirtualHost.Domains,
				Routes:             routes,
				Name:               fmt.Sprintf("%v.%v", ref.Namespace, ref.Name),
				VirtualHostPlugins: vhostPlugins,
			},
			SslConfig: sslConfig,
			Metadata:  ref,
		}
		mergedVirtualServices = append(mergedVirtualServices, mergedVs)
	}

	return mergedVirtualServices
}

func getMergedName(k string) string {
	if k == "" {
		return "catchall"
	}
	return "merged-" + k
}

func getVirtualServiceForGateway(gateway *v1.Gateway, virtualServices v1.VirtualServiceList, resourceErrs reporter.ResourceErrors) v1.VirtualServiceList {
	virtualServiceRefs := gateway.VirtualServices
	// add all virtual services if empty
	if len(gateway.VirtualServices) == 0 {
		for _, virtualService := range virtualServices {
			virtualServiceRefs = append(virtualServiceRefs, core.ResourceRef{
				Name:      virtualService.GetMetadata().Name,
				Namespace: virtualService.GetMetadata().Namespace,
			})
		}
	}

	var virtualServicesForGateway v1.VirtualServiceList
	for _, ref := range virtualServiceRefs {
		virtualService, err := virtualServices.Find(ref.Strings())
		if err != nil {
			resourceErrs.AddError(gateway, err)
			continue
		}
		virtualServicesForGateway = append(virtualServicesForGateway, virtualService)
	}

	return virtualServicesForGateway
}

func filterVirtualServiceForGateway(gateway *v1.Gateway, virtualServices v1.VirtualServiceList) v1.VirtualServiceList {
	var virtualServicesForGateway v1.VirtualServiceList
	for _, virtualService := range virtualServices {
		if gateway.Ssl == hasSsl(virtualService) {
			virtualServicesForGateway = append(virtualServicesForGateway, virtualService)
		}
	}
	return virtualServicesForGateway
}

func hasSsl(vs *v1.VirtualService) bool {
	return vs.SslConfig != nil
}

func desiredListener(gateway *v1.Gateway, virtualServicesForGateway v1.VirtualServiceList) *gloov1.Listener {

	var (
		virtualHosts []*gloov1.VirtualHost
		sslConfigs   []*gloov1.SslConfig
	)

	for _, virtualService := range virtualServicesForGateway {
		ref := virtualService.Metadata.Ref()
		if virtualService.VirtualHost == nil {
			virtualService.VirtualHost = &gloov1.VirtualHost{}
		}
		virtualService.VirtualHost.Name = fmt.Sprintf("%v.%v", ref.Namespace, ref.Name)
		virtualHosts = append(virtualHosts, virtualService.VirtualHost)
		if virtualService.SslConfig != nil {
			sslConfigs = append(sslConfigs, virtualService.SslConfig)
		}
	}
	return &gloov1.Listener{
		Name:        fmt.Sprintf("listener-%s-%d", gateway.BindAddress, gateway.BindPort),
		BindAddress: gateway.BindAddress,
		BindPort:    gateway.BindPort,
		ListenerType: &gloov1.Listener_HttpListener{
			HttpListener: &gloov1.HttpListener{
				VirtualHosts:    virtualHosts,
				ListenerPlugins: gateway.Plugins,
			},
		},
		SslConfiguations: sslConfigs,
		UseProxyProto:    gateway.UseProxyProto,
	}
}
