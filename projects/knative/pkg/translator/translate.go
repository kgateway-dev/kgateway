package translator

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"

	v1alpha1 "github.com/solo-io/gloo/projects/knative/pkg/api/external/knative"
	"knative.dev/serving/pkg/network"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/headers"

	"github.com/pkg/errors"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/retries"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	knativev1alpha1 "knative.dev/serving/pkg/apis/networking/v1alpha1"
)

const (
	bindPortHttp  = 80
	bindPortHttps = 443
)

func translateProxy(ctx context.Context, proxyName, proxyNamespace string, ingresses v1alpha1.IngressList, secrets gloov1.SecretList) (*gloov1.Proxy, error) {
	ingressSpecsByRef := make(map[core.ResourceRef]knativev1alpha1.IngressSpec)
	for _, ing := range ingresses {
		ingressSpecsByRef[ing.GetMetadata().Ref()] = ing.Spec
	}
	return TranslateProxyFromSpecs(ctx, proxyName, proxyNamespace, ingressSpecsByRef, secrets)
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

			if tls.ServerCertificate != "" && tls.ServerCertificate != v1.TLSCertKey {
				contextutils.LoggerFrom(ctx).Warn("Custom ServerCertificate filenames are not currently supported by Gloo")
				continue
			}

			if tls.PrivateKey != "" && tls.PrivateKey != v1.TLSPrivateKeyKey {
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

		for i, rule := range spec.Rules {
			var routes []*gloov1.Route
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
			for _, host := range expandHosts(rule.Hosts) {
				hosts = append(hosts, host)
				if useTls {
					hosts = append(hosts, fmt.Sprintf("%v:%v", host, bindPortHttps))
				} else {
					hosts = append(hosts, fmt.Sprintf("%v:%v", host, bindPortHttp))
				}
			}

			vh := &gloov1.VirtualHost{
				Name:    ing.Key() + "-" + strconv.Itoa(i),
				Domains: hosts,
				Routes:  routes,
			}

			if useTls {
				virtualHostsHttps = append(virtualHostsHttps, vh)
			} else {
				virtualHostsHttp = append(virtualHostsHttp, vh)
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
			Weight:                     weight,
			WeightedDestinationPlugins: weightedDestinationPlugins,
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

// trim kube dns suffixes
// undocumented requirement
// see https://github.com/knative/serving/blob/master/pkg/reconciler/ingress/resources/virtual_service.go#L281
func expandHosts(hosts []string) []string {
	var expanded []string
	allowedSuffixes := []string{
		"",
		"." + network.GetClusterDomainName(),
		".svc." + network.GetClusterDomainName(),
	}
	for _, h := range hosts {
		for _, suffix := range allowedSuffixes {
			if strings.HasSuffix(h, suffix) {
				expanded = append(expanded, strings.TrimSuffix(h, suffix))
			}
		}
	}

	return expanded
}
