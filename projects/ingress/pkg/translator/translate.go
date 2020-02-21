package translator

import (
	"sort"

	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"

	errors "github.com/rotisserie/eris"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/ingress/pkg/api/ingress"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/api/extensions/v1beta1"
)

func translateProxy(namespace string, snap *v1.TranslatorSnapshot, requireIngressClass bool) (*gloov1.Proxy, error) {
	var ingresses []*v1beta1.Ingress
	for _, ig := range snap.Ingresses {
		kubeIngress, err := ingress.ToKube(ig)
		if err != nil {
			return nil, err
		}
		ingresses = append(ingresses, kubeIngress)
	}
	upstreams := snap.Upstreams

	virtualHostsHttp, secureVirtualHosts, err := virtualHosts(ingresses, upstreams, requireIngressClass)
	if err != nil {
		return nil, errors.Wrapf(err, "computing virtual hosts")
	}
	var virtualHostsHttps []*gloov1.VirtualHost
	var sslConfigs []*gloov1.SslConfig
	for _, svh := range secureVirtualHosts {
		svh := svh
		virtualHostsHttps = append(virtualHostsHttps, svh.vh)
		sslConfigs = append(sslConfigs, &gloov1.SslConfig{
			SslSecrets: &gloov1.SslConfig_SecretRef{
				SecretRef: &svh.secret,
			},
			SniDomains: svh.vh.Domains,
		})
	}
	var listeners []*gloov1.Listener
	if len(virtualHostsHttp) > 0 {
		listeners = append(listeners, &gloov1.Listener{
			Name:        "http",
			BindAddress: "::",
			BindPort:    80,
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
			BindPort:    443,
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					VirtualHosts: virtualHostsHttps,
				},
			},
			SslConfigurations: sslConfigs,
		})
	}
	return &gloov1.Proxy{
		Metadata: core.Metadata{
			Name:      "ingress-proxy", // must match envoy role
			Namespace: namespace,
		},
		Listeners: listeners,
	}, nil
}

func upstreamForBackend(upstreams gloov1.UpstreamList, ingressNamespace string, backend v1beta1.IngressBackend) (*gloov1.Upstream, error) {
	// find the upstream with the smallest matching selector
	// longer selectors represent subsets of pods for a service
	var matchingUpstream *gloov1.Upstream
	for _, us := range upstreams {
		switch spec := us.UpstreamType.(type) {
		case *gloov1.Upstream_Kube:
			if spec.Kube.ServiceNamespace == ingressNamespace &&
				spec.Kube.ServiceName == backend.ServiceName &&
				spec.Kube.ServicePort == uint32(backend.ServicePort.IntVal) {
				if matchingUpstream != nil {
					originalSelectorLength := len(matchingUpstream.UpstreamType.(*gloov1.Upstream_Kube).Kube.Selector)
					newSelectorLength := len(spec.Kube.Selector)
					if newSelectorLength > originalSelectorLength {
						continue
					}
				}
				matchingUpstream = us
			}
		}
	}
	if matchingUpstream == nil {
		return nil, errors.Errorf("discovery failure: upstream not found for kube service %v with port %v", backend.ServiceName, backend.ServicePort)
	}
	return matchingUpstream, nil
}

type secureVirtualHost struct {
	vh     *gloov1.VirtualHost
	secret core.ResourceRef
}

func virtualHosts(ingresses []*v1beta1.Ingress, upstreams gloov1.UpstreamList, requireIngressClass bool) ([]*gloov1.VirtualHost, []secureVirtualHost, error) {
	routesByHostHttp := make(map[string][]*gloov1.Route)
	routesByHostHttps := make(map[string][]*gloov1.Route)
	secretsByHost := make(map[string]*core.ResourceRef)
	var defaultBackend *v1beta1.IngressBackend
	for _, ing := range ingresses {
		if requireIngressClass && !isOurIngress(ing) {
			continue
		}
		spec := ing.Spec
		if spec.Backend != nil {
			if defaultBackend != nil {
				log.Warnf("default backend was redeclared in ingress %v, ignoring", ing.Name)
				continue
			}
			defaultBackend = spec.Backend
		}
		for _, tls := range spec.TLS {

			ref := core.ResourceRef{
				Name:      tls.SecretName,
				Namespace: ing.Namespace,
			}
			for _, host := range tls.Hosts {
				if existing, alreadySet := secretsByHost[host]; alreadySet {
					if existing.Name != ref.Name || existing.Namespace != ref.Namespace {
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
				upstream, err := upstreamForBackend(upstreams, ing.Namespace, route.Backend)
				if err != nil {
					return nil, nil, errors.Wrapf(err, "lookup upstream for ingress %v", ing.Name)
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
										Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
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
			Domains: []string{host},
			Routes:  routes,
		})
	}

	for host, routes := range routesByHostHttps {
		glooutils.SortRoutesByPath(routes)
		secret, ok := secretsByHost[host]
		if !ok {
			return nil, nil, errors.Errorf("internal error: secret not found for host %v after processing ingresses", host)
		}
		virtualHostsHttps = append(virtualHostsHttps, secureVirtualHost{
			vh: &gloov1.VirtualHost{
				Name:    host + "-https",
				Domains: []string{host},
				Routes:  routes,
			},
			secret: *secret,
		})
	}

	sort.SliceStable(virtualHostsHttp, func(i, j int) bool {
		return virtualHostsHttp[i].Name < virtualHostsHttp[j].Name
	})
	sort.SliceStable(virtualHostsHttps, func(i, j int) bool {
		return virtualHostsHttps[i].vh.Name < virtualHostsHttps[j].vh.Name
	})
	return virtualHostsHttp, virtualHostsHttps, nil
}

func isOurIngress(ingress *v1beta1.Ingress) bool {
	return ingress.Annotations["kubernetes.io/ingress.class"] == "gloo"
}
