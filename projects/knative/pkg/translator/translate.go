package translator

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/headers"

	knativev1alpha1 "knative.dev/serving/pkg/apis/networking/v1alpha1"
	"github.com/pkg/errors"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/retries"
	v1 "github.com/solo-io/gloo/projects/knative/pkg/api/v1"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	bindPortHttp  = 80
	bindPortHttps = 443
	proxyName     = "knative-proxy"
)

func translateProxy(ctx context.Context, namespace string, snap *v1.TranslatorSnapshot) (*gloov1.Proxy, error) {
	ingresses := make(map[core.ResourceRef]knativev1alpha1.IngressSpec)
	for _, ing := range snap.Ingresses {
		ingresses[ing.GetMetadata().Ref()] = ing.Spec
	}
	return TranslateProxyFromSpecs(ctx, proxyName, namespace, ingresses, snap.Secrets)
}

// made public to be shared with the (soon to be deprecated) clusteringress controller
func TranslateProxyFromSpecs(ctx context.Context, proxyName, proxyNamespace string, ingresses map[core.ResourceRef]knativev1alpha1.IngressSpec, secrets gloov1.SecretList) (*gloov1.Proxy, error) {
	virtualHostsHttp, virtualHostsHttps, sslConfigs, err := routingConfig(ctx, ingresses, secrets)
	if err != nil {
		return nil, errors.Wrapf(err, "computing virtual hosts")
	}
	var listeners []*gloov1.Listener
	if len(virtualHostsHttp) > 0 {
		listeners = append(listeners, &gloov1.Listener{
			Name:        "http",
			BindAddress: "::",
			BindPort:    bindPortHttp,
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
			BindPort:    bindPortHttps,
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
			Name:      proxyName, // must match envoy role
			Namespace: proxyNamespace,
		},
		Listeners: listeners,
	}, nil
}

func routingConfig(ctx context.Context, ingresses map[core.ResourceRef]knativev1alpha1.IngressSpec, secrets gloov1.SecretList) ([]*gloov1.VirtualHost, []*gloov1.VirtualHost, []*gloov1.SslConfig, error) {

	var virtualHostsHttp, virtualHostsHttps []*gloov1.VirtualHost
	var sslConfigs []*gloov1.SslConfig
	for ing, spec := range ingresses {

		for _, tls := range spec.TLS {
			secret, err := secrets.Find(tls.SecretNamespace, tls.SecretName)
			if err != nil {
				return nil, nil, nil, errors.Wrapf(err, "invalid secret for knative ingress %v", ing.Name)
			}

			ref := secret.Metadata.Ref()

			if tls.ServerCertificate != "" {
				contextutils.LoggerFrom(ctx).Warn("Custom ServerCertificate filenames are not currently supported by Gloo")
				continue
			}

			if tls.PrivateKey != "" {
				contextutils.LoggerFrom(ctx).Warn("Custom PrivateKey filenames are not currently supported by Gloo")
				continue
			}

			sslConfigs = append(sslConfigs, &gloov1.SslConfig{
				SniDomains: tls.Hosts,
				SslSecrets: &gloov1.SslConfig_SecretRef{
					SecretRef: &ref,
				},
			})
		}

		var routes []*gloov1.Route
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

				action, err := routeActionFromSplits(route.Splits)
				if err != nil {
					return nil, nil, nil, errors.Wrapf(err, "")
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
						HeaderManipulation: getHeaderManipulation(route.AppendHeaders),
						Timeout:            timeout,
						Retries:            retryPolicy,
					},
				}
				routes = append(routes, route)

			}
			useTls := len(spec.TLS) > 0

			var hosts []string
			for _, host := range rule.Hosts {
				hosts = append(hosts, host)
				if useTls {
					hosts = append(hosts, fmt.Sprintf("%v:%v", host, bindPortHttps))
				} else {
					hosts = append(hosts, fmt.Sprintf("%v:%v", host, bindPortHttp))
				}
			}

			if useTls {
				virtualHostsHttps = append(virtualHostsHttps, &gloov1.VirtualHost{
					Name:    ing.Key(),
					Domains: hosts,
					Routes:  routes,
				})
			} else {
				virtualHostsHttp = append(virtualHostsHttp, &gloov1.VirtualHost{
					Name:    ing.Key(),
					Domains: hosts,
					Routes:  routes,
				})
			}
		}
	}

	sort.SliceStable(virtualHostsHttp, func(i, j int) bool {
		return virtualHostsHttp[i].Name < virtualHostsHttp[j].Name
	})
	sort.SliceStable(virtualHostsHttps, func(i, j int) bool {
		return virtualHostsHttps[i].Name < virtualHostsHttps[j].Name
	})
	return virtualHostsHttp, virtualHostsHttps, sslConfigs, nil
}

func routeActionFromSplits(splits []knativev1alpha1.IngressBackendSplit) (*gloov1.RouteAction, error) {
	switch len(splits) {
	case 0:
		return nil, errors.Errorf("invalid cluster ingress: must provide at least 1 split")
	}

	var destinations []*gloov1.WeightedDestination
	for _, split := range splits {
		var weightedDestinationPlugins *gloov1.WeightedDestinationPlugins
		if headerManipulaion := getHeaderManipulation(split.AppendHeaders); headerManipulaion != nil {
			weightedDestinationPlugins = &gloov1.WeightedDestinationPlugins{
				HeaderManipulation: headerManipulaion,
			}
		}
		weight := uint32(split.Percent)
		if len(splits) == 1 {
			weight = 100
		}
		destinations = append(destinations, &gloov1.WeightedDestination{
			Destination: &gloov1.Destination{
				DestinationType: serviceForSplit(split),
			},
			Weight:                    weight,
			WeighedDestinationPlugins: weightedDestinationPlugins,
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

func serviceForSplit(split knativev1alpha1.IngressBackendSplit) *gloov1.Destination_Kube {
	return &gloov1.Destination_Kube{
		Kube: &gloov1.KubernetesServiceDestination{
			Ref:  core.ResourceRef{Name: split.ServiceName, Namespace: split.ServiceNamespace},
			Port: uint32(split.ServicePort.IntValue()),
		},
	}
}

func getHeaderManipulation(headersToAppend map[string]string) *headers.HeaderManipulation {
	if len(headersToAppend) == 0 {
		return nil
	}
	var headersToAdd []*headers.HeaderValueOption
	for name, value := range headersToAppend {
		headersToAdd = append(headersToAdd, &headers.HeaderValueOption{Header: &headers.HeaderValue{Key: name, Value: value}})
	}
	return &headers.HeaderManipulation{
		RequestHeadersToAdd: headersToAdd,
	}
}
