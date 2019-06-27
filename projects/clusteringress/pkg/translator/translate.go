package translator

import (
	"sort"
	"time"

	knativev1alpha1 "github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/utils"
	v1alpha1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/external/knative"
	v1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/retries"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/transformation"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func translateProxy(namespace string, snap *v1.TranslatorSnapshot) (*gloov1.Proxy, error) {
	clusterIngresses := snap.Clusteringresses
	upstreams := snap.Upstreams
	secrets := snap.Secrets

	virtualHostsHttp, secureVirtualHosts, err := virtualHosts(clusterIngresses, upstreams, secrets)
	if err != nil {
		return nil, errors.Wrapf(err, "computing virtual hosts")
	}
	var virtualHostsHttps []*gloov1.VirtualHost
	var sslConfigs []*gloov1.SslConfig
	for _, svh := range secureVirtualHosts {
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
			Name:      "clusteringress-proxy", // must match envoy role
			Namespace: namespace,
		},
		Listeners: listeners,
	}, nil
}

type secureVirtualHost struct {
	vh     *gloov1.VirtualHost
	secret core.ResourceRef
}

func virtualHosts(ingresses v1alpha1.ClusterIngressList, upstreams gloov1.UpstreamList, secrets gloov1.SecretList) ([]*gloov1.VirtualHost, []secureVirtualHost, error) {
	routesByHostHttp := make(map[string][]*gloov1.Route)
	routesByHostHttps := make(map[string][]*gloov1.Route)
	secretsByHost := make(map[string]*core.ResourceRef)
	for _, ing := range ingresses {
		spec := ing.Spec
		for _, tls := range spec.TLS {
			secret, err := secrets.Find(ing.Namespace, tls.SecretName)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "invalid secret for knative ingress %v", ing.Name)
			}

			ref := secret.Metadata.Ref()
			for _, host := range tls.Hosts {
				if existing, alreadySet := secretsByHost[host]; alreadySet {
					if existing.Name != ref.Name || existing.Namespace != ref.Namespace {
						log.Warnf("a TLS secret for host %v was redefined in knative ingress %v, ignoring", ing.Name)
						continue
					}
				}
				secretsByHost[host] = &ref
			}
		}

		for i, rule := range spec.Rules {
			if rule.HTTP == nil {
				log.Warnf("rule %v in knative ingress %v is missing HTTP field", i, ing.Name)
				continue
			}
			for _, route := range rule.HTTP.Paths {
				pathRegex := route.Path
				if pathRegex == "" {
					pathRegex = ".*"
				}
				appendHeaders := make(map[string]*transformation.InjaTemplate)
				for name, value := range route.AppendHeaders {
					appendHeaders[name] = &transformation.InjaTemplate{Text: value}
				}
				var timeout *time.Duration
				if route.Timeout != nil {
					timeout = &route.Timeout.Duration
				}
				var retryPolicy *retries.RetryPolicy
				if route.Retries != nil {
					var perTryTimeout *time.Duration
					if route.Retries.PerTryTimeout != nil {
						perTryTimeout = &route.Retries.PerTryTimeout.Duration
					}
					retryPolicy = &retries.RetryPolicy{
						NumRetries:    uint32(route.Retries.Attempts),
						PerTryTimeout: perTryTimeout,
					}
				}
				appendHeadersTransformation := &transformation.RouteTransformations{
					RequestTransformation: &transformation.Transformation{
						TransformationType: &transformation.Transformation_TransformationTemplate{
							TransformationTemplate: &transformation.TransformationTemplate{
								Headers: appendHeaders,
								BodyTransformation: &transformation.TransformationTemplate_Passthrough{
									Passthrough: &transformation.Passthrough{},
								},
							},
						},
					},
				}

				action, err := routeActionFromSplits(route.Splits, upstreams)
				if err != nil {
					return nil, nil, errors.Wrapf(err, "")
				}

				route := &gloov1.Route{
					Matcher: &gloov1.Matcher{
						PathSpecifier: &gloov1.Matcher_Regex{
							Regex: pathRegex,
						},
					},
					Action: &gloov1.Route_RouteAction{
						RouteAction: action,
					},
					RoutePlugins: &gloov1.RoutePlugins{
						Transformations: appendHeadersTransformation,
						Timeout:         timeout,
						Retries:         retryPolicy,
					},
				}
				for _, host := range rule.Hosts {
					if _, useTls := secretsByHost[host]; useTls {
						routesByHostHttps[host] = append(routesByHostHttps[host], route)
					} else {
						routesByHostHttp[host] = append(routesByHostHttp[host], route)
					}
				}
			}
		}
	}

	var virtualHostsHttp []*gloov1.VirtualHost
	var virtualHostsHttps []secureVirtualHost

	// TODO (ilackarms): support for VirtualHostPlugins on ingress?
	for host, routes := range routesByHostHttp {
		sortByLongestPathName(routes)
		virtualHostsHttp = append(virtualHostsHttp, &gloov1.VirtualHost{
			Name:    host + "-http",
			Domains: []string{host},
			Routes:  routes,
		})
	}

	for host, routes := range routesByHostHttps {
		sortByLongestPathName(routes)
		secret, ok := secretsByHost[host]
		if !ok {
			return nil, nil, errors.Errorf("internal error: secret not found for host %v after processing knative ingresses", host)
		}
		virtualHostsHttps = append(virtualHostsHttps, secureVirtualHost{
			vh: &gloov1.VirtualHost{
				Name:    host + "-http",
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

func routeActionFromSplits(splits []knativev1alpha1.IngressBackendSplit, upstreams gloov1.UpstreamList) (*gloov1.RouteAction, error) {
	switch len(splits) {
	case 0:
		return nil, errors.Errorf("invalid cluster ingress: must provide at least 1 split")
	case 1:
		split := splits[0]
		upstream, err := upstreamForSplit(upstreams, split)
		if err != nil {
			return nil, errors.Wrapf(err, "getting upstream for split %v", split)
		}
		return &gloov1.RouteAction{
			Destination: &gloov1.RouteAction_Single{
				Single: &gloov1.Destination{
					DestinationType: &gloov1.Destination_Upstream{
						Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
					},
				},
			},
		}, nil
	}

	var destinations []*gloov1.WeightedDestination
	for _, split := range splits {
		upstream, err := upstreamForSplit(upstreams, split)
		if err != nil {
			return nil, errors.Wrapf(err, "getting upstream for split %v", split)
		}
		destinations = append(destinations, &gloov1.WeightedDestination{
			Destination: &gloov1.Destination{
				DestinationType: &gloov1.Destination_Upstream{
					Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
				},
			},
			Weight: uint32(split.Percent),
		})
	}
	return &gloov1.RouteAction{
		Destination: &gloov1.RouteAction_Multi{
			Multi: &gloov1.MultiDestination{
				Destinations: destinations,
			},
		},
	}, nil
}

func upstreamForSplit(upstreams gloov1.UpstreamList, backend knativev1alpha1.IngressBackendSplit) (*gloov1.Upstream, error) {
	// find the upstream with the smallest matching selector
	// longer selectors represent subsets of pods for a service
	var matchingUpstream *gloov1.Upstream
	for _, us := range upstreams {
		switch spec := us.UpstreamSpec.UpstreamType.(type) {
		case *gloov1.UpstreamSpec_Kube:
			if spec.Kube.ServiceNamespace == backend.ServiceNamespace &&
				spec.Kube.ServiceName == backend.ServiceName &&
				spec.Kube.ServicePort == uint32(backend.ServicePort.IntVal) {
				if matchingUpstream != nil {
					originalSelectorLength := len(matchingUpstream.UpstreamSpec.UpstreamType.(*gloov1.UpstreamSpec_Kube).Kube.Selector)
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

func sortByLongestPathName(routes []*gloov1.Route) {
	sort.SliceStable(routes, func(i, j int) bool {
		return routes[i].Matcher.PathSpecifier.(*gloov1.Matcher_Regex).Regex > routes[j].Matcher.PathSpecifier.(*gloov1.Matcher_Regex).Regex
	})
}
