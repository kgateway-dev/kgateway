package translator

import (
	"context"
	"sort"

	"github.com/solo-io/gloo/projects/ingress/pkg/api/service"
	"github.com/solo-io/go-utils/contextutils"
	kubev1 "k8s.io/api/core/v1"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"

	errors "github.com/rotisserie/eris"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/ingress/pkg/api/ingress"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	networkingv1 "k8s.io/api/networking/v1"
)

const defaultIngressClass = "gloo"

const IngressClassKey = "kubernetes.io/ingress.class"

func translateProxy(ctx context.Context, namespace string, snap *v1.TranslatorSnapshot, requireIngressClass bool, ingressClass string) *gloov1.Proxy {

	if ingressClass == "" {
		ingressClass = defaultIngressClass
	}

	var ingresses []*networkingv1.Ingress
	for _, ig := range snap.Ingresses {
		kubeIngress, err := ingress.ToKube(ig)
		if err != nil {
			contextutils.LoggerFrom(ctx).Errorf("internal error: parsing internal ingress representation: %v", err)
			continue
		}
		ingresses = append(ingresses, kubeIngress)
	}

	var services []*kubev1.Service
	for _, svc := range snap.Services {
		kubeSvc, err := service.ToKube(svc)
		if err != nil {
			contextutils.LoggerFrom(ctx).Errorf("internal error: parsing internal service representation: %v", err)
			continue
		}
		services = append(services, kubeSvc)
	}

	upstreams := snap.Upstreams

	virtualHostsHttp, secureVirtualHosts := virtualHosts(ctx, ingresses, upstreams, services, requireIngressClass, ingressClass)

	var virtualHostsHttps []*gloov1.VirtualHost
	var sslConfigs []*ssl.SslConfig
	for _, svh := range secureVirtualHosts {
		svh := svh
		virtualHostsHttps = append(virtualHostsHttps, svh.vh)
		sslConfigs = append(sslConfigs, &ssl.SslConfig{
			SslSecrets: &ssl.SslConfig_SecretRef{
				SecretRef: &svh.secret,
			},
			SniDomains: svh.vh.GetDomains(),
		})
	}
	var listeners []*gloov1.Listener
	if len(virtualHostsHttp) > 0 {
		listeners = append(listeners, &gloov1.Listener{
			Name:        "http",
			BindAddress: "::",
			BindPort:    8080,
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					VirtualHosts: virtualHostsHttp,
				},
			},
		})
	}
	if len(virtualHostsHttps) > 0 {
		listeners = append(listeners, &gloov1.Listener{
			Name:        "https",
			BindAddress: "::",
			BindPort:    8443,
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					VirtualHosts: virtualHostsHttps,
				},
			},
			SslConfigurations: sslConfigs,
		})
	}
	return &gloov1.Proxy{
		Metadata: &core.Metadata{
			Name:      "ingress-proxy", // must match envoy role
			Namespace: namespace,
		},
		Listeners: listeners,
	}
}

func upstreamForBackend(upstreams gloov1.UpstreamList, services []*kubev1.Service, ingressNamespace string, backend networkingv1.IngressBackend) (*gloov1.Upstream, error) {
	serviceName, servicePort, err := getServiceNameAndPort(services, ingressNamespace, backend.Service)
	if err != nil {
		return nil, err
	}

	// find the upstream with the smallest matching selector
	// longer selectors represent subsets of pods for a service
	var matchingUpstream *gloov1.Upstream
	for _, us := range upstreams {
		switch spec := us.GetUpstreamType().(type) {
		case *gloov1.Upstream_Kube:
			if spec.Kube.GetServiceNamespace() == ingressNamespace &&
				spec.Kube.GetServiceName() == serviceName &&
				spec.Kube.GetServicePort() == uint32(servicePort) {
				if matchingUpstream != nil {
					originalSelectorLength := len(matchingUpstream.GetUpstreamType().(*gloov1.Upstream_Kube).Kube.GetSelector())
					newSelectorLength := len(spec.Kube.GetSelector())
					if newSelectorLength > originalSelectorLength {
						continue
					}
				}
				matchingUpstream = us
			}
		}
	}
	if matchingUpstream == nil {
		return nil, errors.Errorf("discovery failure: upstream not found for kube service %v with port %v", serviceName, servicePort)
	}
	return matchingUpstream, nil
}

// getServiceNameAndPort returns the service name and port number for an IngressServiceBackend or an error if the
// defined IngressServiceBackend does not match any available services.
// An IngressServiceBackend can have have its port defined either by number or name, so we must handle both cases
func getServiceNameAndPort(services []*kubev1.Service, namespace string, ingressService *networkingv1.IngressServiceBackend) (string, int32, error) {
	if ingressService == nil {
		return "", 0, errors.New("no service specified for ingress backend")
	}
	serviceName := ingressService.Name
	// If the IngressServiceBackend defines a named port, we first find the service by name/namespace
	// and then determine the port number which maps to the port name
	if ingressService.Port.Name != "" {
		portName := ingressService.Port.Name
		for _, svc := range services {
			if svc.Name == serviceName && svc.Namespace == namespace {
				for _, port := range svc.Spec.Ports {
					if port.Name == portName {
						return serviceName, port.Port, nil
					}
				}
				return "", 0, errors.Errorf("port %v not found for service %v.%v", portName, serviceName, namespace)
			}
		}
		return "", 0, errors.Errorf("service %v.%v not found", serviceName, namespace)
	}

	return serviceName, ingressService.Port.Number, nil
}

type secureVirtualHost struct {
	vh     *gloov1.VirtualHost
	secret core.ResourceRef
}

func virtualHosts(ctx context.Context, ingresses []*networkingv1.Ingress, upstreams gloov1.UpstreamList, services []*kubev1.Service, requireIngressClass bool, ingressClass string) ([]*gloov1.VirtualHost, []secureVirtualHost) {
	routesByHostHttp := make(map[string][]*gloov1.Route)
	routesByHostHttps := make(map[string][]*gloov1.Route)
	secretsByHost := make(map[string]*core.ResourceRef)
	var defaultBackend *networkingv1.IngressBackend
	for _, ing := range ingresses {
		if requireIngressClass && !isOurIngress(ing, ingressClass) {
			continue
		}
		spec := ing.Spec
		if spec.DefaultBackend != nil {
			if defaultBackend != nil {
				contextutils.LoggerFrom(ctx).Warnf("default backend was redeclared in ingress %v, ignoring", ing.Name)
				continue
			}
			defaultBackend = spec.DefaultBackend
		}
		for _, tls := range spec.TLS {

			ref := core.ResourceRef{
				Name:      tls.SecretName,
				Namespace: ing.Namespace,
			}
			for _, host := range tls.Hosts {
				if existing, alreadySet := secretsByHost[host]; alreadySet {
					if existing.GetName() != ref.GetName() || existing.GetNamespace() != ref.GetNamespace() {
						log.Warnf("a TLS secret for host %v was redefined in ingress %v, ignoring", ing.Name)
						continue
					}
				}
				secretsByHost[host] = &ref
			}
		}

		for i, rule := range spec.Rules {
			host := rule.Host
			if host == "" {
				host = "*"
			}
			// set a "default route"
			if rule.HTTP == nil {
				log.Warnf("rule %v in ingress %v is missing HTTP field", i, ing.Name)
				continue
			}
			for _, route := range rule.HTTP.Paths {
				upstream, err := upstreamForBackend(upstreams, services, ing.Namespace, route.Backend)
				if err != nil {
					contextutils.LoggerFrom(ctx).Errorf("lookup upstream for ingress %v: %v", ing.Name, err)
					continue
				}

				pathRegex := route.Path
				if pathRegex == "" {
					pathRegex = ".*"
				}
				route := &gloov1.Route{
					Matchers: []*matchers.Matcher{{
						PathSpecifier: &matchers.Matcher_Regex{
							Regex: pathRegex,
						},
					}},
					Action: &gloov1.Route_RouteAction{
						RouteAction: &gloov1.RouteAction{
							Destination: &gloov1.RouteAction_Single{
								Single: &gloov1.Destination{
									DestinationType: &gloov1.Destination_Upstream{
										Upstream: upstream.GetMetadata().Ref(),
									},
								},
							},
						},
					},
				}
				if _, useTls := secretsByHost[host]; useTls {
					routesByHostHttps[host] = append(routesByHostHttps[host], route)
				} else {
					routesByHostHttp[host] = append(routesByHostHttp[host], route)
				}
			}
		}
	}

	var virtualHostsHttp []*gloov1.VirtualHost
	var virtualHostsHttps []secureVirtualHost

	for host, routes := range routesByHostHttp {
		glooutils.SortRoutesByPath(routes)
		virtualHostsHttp = append(virtualHostsHttp, &gloov1.VirtualHost{
			Name:    host + "-http",
			Domains: []string{host, host + ":8080"},
			Routes:  routes,
		})
	}

	for host, routes := range routesByHostHttps {
		glooutils.SortRoutesByPath(routes)
		secret, ok := secretsByHost[host]
		if !ok {
			contextutils.LoggerFrom(ctx).Errorf("internal error: secret not found for host %v after processing ingresses", host)
			continue
		}
		virtualHostsHttps = append(virtualHostsHttps, secureVirtualHost{
			vh: &gloov1.VirtualHost{
				Name:    host + "-https",
				Domains: []string{host, host + ":8443"},
				Routes:  routes,
			},
			secret: *secret,
		})
	}

	sort.SliceStable(virtualHostsHttp, func(i, j int) bool {
		return virtualHostsHttp[i].GetName() < virtualHostsHttp[j].GetName()
	})
	sort.SliceStable(virtualHostsHttps, func(i, j int) bool {
		return virtualHostsHttps[i].vh.GetName() < virtualHostsHttps[j].vh.GetName()
	})
	return virtualHostsHttp, virtualHostsHttps
}

func isOurIngress(ingress *networkingv1.Ingress, ingressClassToUse string) bool {
	return ingress.Annotations[IngressClassKey] == ingressClassToUse
}
