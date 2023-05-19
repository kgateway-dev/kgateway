package translator

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/golang/protobuf/ptypes/wrappers"

	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"

	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/pkg/network"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"k8s.io/apimachinery/pkg/util/sets"

	v1alpha1 "github.com/solo-io/gloo/projects/knative/pkg/api/external/knative"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"

	errors "github.com/rotisserie/eris"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	knativev1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
)

const (
	ingressClassAnnotation = networking.IngressClassAnnotationKey
	glooIngressClass       = "gloo.ingress.networking.knative.dev"
)

const (
	bindPortHttp  = 8080
	bindPortHttps = 8443

	// a comma-separated list of sni domains
	sslAnnotationKeySniDomains = "gloo.networking.knative.dev/ssl.sni_domains"
	// the name of the secret containing tls certs
	sslAnnotationKeySecretName = "gloo.networking.knative.dev/ssl.secret_name"
	// the namespace of the secret containing tls certs
	// defaults to the ingress' namespace
	sslAnnotationKeySecretNamespace = "gloo.networking.knative.dev/ssl.secret_namespace"
)

func sslConfigFromAnnotations(annotations map[string]string, namespace string) *ssl.SslConfig {
	secretName, ok := annotations[sslAnnotationKeySecretName]
	if !ok {
		return nil
	}

	secretNamespace, ok := annotations[sslAnnotationKeySecretNamespace]
	if !ok {
		secretNamespace = namespace
	}

	sniDomains := strings.Split(annotations[sslAnnotationKeySniDomains], ",")

	return &ssl.SslConfig{
		SslSecrets: &ssl.SslConfig_SecretRef{
			SecretRef: &core.ResourceRef{
				Name:      secretName,
				Namespace: secretNamespace,
			},
		},
		SniDomains: sniDomains,
	}
}

func translateProxy(ctx context.Context, proxyName, proxyNamespace string, ingresses v1alpha1.IngressList) (*gloov1.Proxy, error) {
	// use map of *core.Metadata to support both Ingress and ClusterIngress,
	// which share the same Spec type
	ingressSpecsByRef := make(map[*core.Metadata]knativev1alpha1.IngressSpec)
	for _, ing := range ingresses {
		meta := ing.GetMetadata()
		ingressSpecsByRef[meta] = ing.Spec
	}
	return TranslateProxyFromSpecs(ctx, proxyName, proxyNamespace, ingressSpecsByRef)
}

// made public to be shared with the (soon to be deprecated) clusteringress controller
func TranslateProxyFromSpecs(ctx context.Context, proxyName, proxyNamespace string, ingresses map[*core.Metadata]knativev1alpha1.IngressSpec) (*gloov1.Proxy, error) {
	virtualHostsHttp, virtualHostsHttps, sslConfigs, err := routingConfig(ctx, ingresses)
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
		Metadata: &core.Metadata{
			Name:      proxyName, // must match envoy role
			Namespace: proxyNamespace,
		},
		Listeners: listeners,
	}, nil
}

func routingConfig(ctx context.Context, ingresses map[*core.Metadata]knativev1alpha1.IngressSpec) ([]*gloov1.VirtualHost, []*gloov1.VirtualHost, []*ssl.SslConfig, error) {

	var virtualHostsHttp, virtualHostsHttps []*gloov1.VirtualHost
	var sslConfigs []*ssl.SslConfig
	for ing, spec := range ingresses {

		for _, tls := range spec.TLS {
			secretNamespace := tls.SecretNamespace
			if secretNamespace == "" {
				// default to namespace shared with ingress
				secretNamespace = ing.GetNamespace()
			}

			sslConfigs = append(sslConfigs, &ssl.SslConfig{
				SniDomains: tls.Hosts,
				SslSecrets: &ssl.SslConfig_SecretRef{
					// pass secret through to gloo,
					// allow Gloo to perform secret validation
					SecretRef: &core.ResourceRef{
						Namespace: secretNamespace,
						Name:      tls.SecretName,
					},
				},
			})
		}

		// use tls if spec contains tls, or user sets with annotations
		useTls := len(spec.TLS) > 0

		if customSsl := sslConfigFromAnnotations(ing.GetAnnotations(), ing.GetNamespace()); customSsl != nil {
			useTls = true
			sslConfigs = append(sslConfigs, customSsl)
		}

		for i, rule := range spec.Rules {
			var routes []*gloov1.Route
			if rule.HTTP == nil {
				log.Warnf("rule %v in knative ingress %v is missing HTTP field", i, ing.GetName())
				continue
			}
			for _, route := range rule.HTTP.Paths {
				pathRegex := route.Path
				if pathRegex == "" {
					pathRegex = ".*"
				}

				action, err := routeActionFromSplits(route.Splits)
				if err != nil {
					return nil, nil, nil, errors.Wrapf(err, "")
				}

				route := &gloov1.Route{
					Matchers: []*matchers.Matcher{{
						PathSpecifier: &matchers.Matcher_Regex{
							Regex: pathRegex,
						},
					}},
					Action: &gloov1.Route_RouteAction{
						RouteAction: action,
					},
					Options: &gloov1.RouteOptions{
						HeaderManipulation: getHeaderManipulation(route.AppendHeaders),
					},
				}
				routes = append(routes, route)

			}

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
				Name:    ing.Ref().Key() + "-" + strconv.Itoa(i),
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
		return virtualHostsHttp[i].GetName() < virtualHostsHttp[j].GetName()
	})
	sort.SliceStable(virtualHostsHttps, func(i, j int) bool {
		return virtualHostsHttps[i].GetName() < virtualHostsHttps[j].GetName()
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
		var weightedDestinationPlugins *gloov1.WeightedDestinationOptions
		if headerManipulaion := getHeaderManipulation(split.AppendHeaders); headerManipulaion != nil {
			weightedDestinationPlugins = &gloov1.WeightedDestinationOptions{
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
			Weight:  &wrappers.UInt32Value{Value: weight},
			Options: weightedDestinationPlugins,
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
			Ref:  &core.ResourceRef{Name: split.ServiceName, Namespace: split.ServiceNamespace},
			Port: uint32(split.ServicePort.IntValue()),
		},
	}
}

func getHeaderManipulation(headersToAppend map[string]string) *headers.HeaderManipulation {
	if len(headersToAppend) == 0 {
		return nil
	}
	var headersToAdd []*envoycore_sk.HeaderValueOption
	for name, value := range headersToAppend {
		headersToAdd = append(headersToAdd, &envoycore_sk.HeaderValueOption{HeaderOption: &envoycore_sk.HeaderValueOption_Header{Header: &envoycore_sk.HeaderValue{Key: name, Value: value}}})
	}
	return &headers.HeaderManipulation{
		RequestHeadersToAdd: headersToAdd,
	}
}

// trim kube dns suffixes
// undocumented requirement
// see https://github.com/knative/serving/blob/main/pkg/reconciler/ingress/resources/virtual_service.go#L281
func expandHosts(hosts []string) []string {
	expanded := sets.NewString()
	allowedSuffixes := []string{
		"",
		"." + network.GetClusterDomainName(),
		".svc." + network.GetClusterDomainName(),
	}
	for _, h := range hosts {
		for _, suffix := range allowedSuffixes {
			if strings.HasSuffix(h, suffix) {
				expanded.Insert(strings.TrimSuffix(h, suffix))
			}
		}
	}

	return expanded.List()
}
