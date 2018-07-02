package translator

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	defaultv1 "github.com/solo-io/gloo/pkg/api/defaults/v1"

	"github.com/solo-io/gloo/pkg/control-plane/snapshot"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

func (t *Translator) computeRouteConfig(role *v1.Role, listenerName string, routeCfgName string, inputs *snapshot.Cache, configErrs configErrors) *envoyapi.RouteConfiguration {
	virtualHosts := t.computeVirtualHosts(role, listenerName, inputs, configErrs)

	return &envoyapi.RouteConfiguration{
		Name:         routeCfgName,
		VirtualHosts: virtualHosts,
	}
}

func (t *Translator) computeVirtualHosts(role *v1.Role, listenerName string, inputs *snapshot.Cache, configErrs configErrors) []envoyroute.VirtualHost {
	if err := validateVirtualServiceDomains(inputs.Cfg.VirtualServices); err != nil {
		configErrs.addError(role, errors.Wrapf(err, "invalid listener %v", listenerName))
	}
	var virtualHosts []envoyroute.VirtualHost
	for _, virtualService := range inputs.Cfg.VirtualServices {
		virtualHosts = append(virtualHosts, t.computeVirtualHost(virtualService, inputs, configErrs))
	}
	return virtualHosts
}

func (t *Translator) computeVirtualHost(virtualService *v1.VirtualService, inputs *snapshot.Cache, configErrs configErrors) envoyroute.VirtualHost {
	var envoyRoutes []envoyroute.Route
	for _, route := range virtualService.Routes {
		if err := validateRouteDestinations(inputs.Cfg.Upstreams, route); err != nil {
			configErrs.addError(virtualService, err)
		}
		out := envoyroute.Route{}
		for _, plug := range t.plugins {
			routePlugin, ok := plug.(plugins.RoutePlugin)
			if !ok {
				continue
			}
			params := &plugins.RoutePluginParams{
				Upstreams: inputs.Cfg.Upstreams,
			}
			if err := routePlugin.ProcessRoute(params, route, &out); err != nil {
				configErrs.addError(virtualService, err)
			}
		}
		envoyRoutes = append(envoyRoutes, out)
	}
	// validate ssl config if the host specifies one
	if err := validateVirtualServiceSSLConfig(virtualService, inputs.Secrets); err != nil {
		configErrs.addError(virtualService, err)
	}
	domains := virtualService.Domains
	if len(domains) == 0 || (len(domains) == 1 && domains[0] == "") {
		domains = []string{"*"}
	}
	var requireTls envoyroute.VirtualHost_TlsRequirementType
	if virtualService.SslConfig != nil {
		// TODO (ilackarms): support external-only TLS
		requireTls = envoyroute.VirtualHost_ALL
	}

	return envoyroute.VirtualHost{
		Name:       virtualHostName(virtualService.Name),
		Domains:    domains,
		Routes:     envoyRoutes,
		RequireTls: requireTls,
		//TODO (ilackarms):
		//VirtualClusters: nil,
		//RateLimits: nil,
		//RequestHeadersToAdd: nil,
		//ResponseHeadersToRemove: nil,
		//Cors: nil,
		//Auth: nil,
	}
}

// returns an error if any of the virtualservice domains overlap
func validateVirtualServiceDomains(virtualServices []*v1.VirtualService) error {
	// this shouldbe a 1-1 mapping
	// if len(domainsToVirtualServices[domain]) > 1, it's an error
	domainsToVirtualServices := make(map[string][]string)
	for _, vService := range virtualServices {
		if len(vService.Domains) == 0 {
			// default virtualservice
			domainsToVirtualServices["*"] = append(domainsToVirtualServices["*"], vService.Name)
		}
		for _, domain := range vService.Domains {
			// default virtualservice can be specified with empty string
			if domain == "" {
				domain = "*"
			}
			domainsToVirtualServices[domain] = append(domainsToVirtualServices[domain], vService.Name)
		}
	}
	var domainErrors error
	// see if we found any conflicts, if so, write reports
	for domain, vServices := range domainsToVirtualServices {
		if len(vServices) > 1 {
			domainErrors = multierror.Append(domainErrors, errors.Errorf("domain %v is "+
				"shared by the following virtual services: %v", domain, vServices))
		}
	}
	return domainErrors
}

func validateRouteDestinations(upstreams []*v1.Upstream, route *v1.Route) error {
	// collect existing upstreams/functions for matching
	upstreamsAndTheirFunctions := make(map[string][]string)

	for _, upstream := range upstreams {
		// don't consider errored upstreams to be valid destinations
		var funcsForUpstream []string
		for _, fn := range upstream.Functions {
			funcsForUpstream = append(funcsForUpstream, fn.Name)
		}
		upstreamsAndTheirFunctions[upstream.Name] = funcsForUpstream
	}

	// make sure the destination itself has the right structure
	switch {
	case route.SingleDestination != nil && len(route.MultipleDestinations) == 0:
		return validateSingleDestination(upstreamsAndTheirFunctions, route.SingleDestination)
	case route.SingleDestination == nil && len(route.MultipleDestinations) > 0:
		return validateMultiDestination(upstreamsAndTheirFunctions, route.MultipleDestinations)
	}
	return errors.Errorf("must specify either 'single_destination' or 'multiple_destinations' for route")
}

func validateMultiDestination(upstreamsAndTheirFunctions map[string][]string, destinations []*v1.WeightedDestination) error {
	for _, dest := range destinations {
		if err := validateSingleDestination(upstreamsAndTheirFunctions, dest.Destination); err != nil {
			return errors.Wrap(err, "invalid destination in weighted destination list")
		}
	}
	return nil
}

func validateSingleDestination(upstreamsAndTheirFunctions map[string][]string, destination *v1.Destination) error {
	switch dest := destination.DestinationType.(type) {
	case *v1.Destination_Upstream:
		return validateUpstreamDestination(upstreamsAndTheirFunctions, dest)
	case *v1.Destination_Function:
		return validateFunctionDestination(upstreamsAndTheirFunctions, dest)
	}
	return errors.New("must specify either a function or upstream on a single destination")
}

func validateUpstreamDestination(upstreamsAndTheirFunctions map[string][]string, upstreamDestination *v1.Destination_Upstream) error {
	upstreamName := upstreamDestination.Upstream.Name
	if _, ok := upstreamsAndTheirFunctions[upstreamName]; !ok {
		return errors.Errorf("upstream %v was not found or had errors for upstream destination", upstreamName)
	}
	return nil
}

func validateFunctionDestination(upstreamsAndTheirFunctions map[string][]string, functionDestination *v1.Destination_Function) error {
	upstreamName := functionDestination.Function.UpstreamName
	upstreamFuncs, ok := upstreamsAndTheirFunctions[upstreamName]
	if !ok {
		return errors.Errorf("upstream %v was not found or had errors for function destination", upstreamName)
	}
	functionName := functionDestination.Function.FunctionName
	if !stringInSlice(upstreamFuncs, functionName) {
		log.Warnf("function %v/%v was not found for function destination", upstreamName, functionName)
	}
	return nil
}

func stringInSlice(slice []string, s string) bool {
	for _, el := range slice {
		if el == s {
			return true
		}
	}
	return false
}

func validateVirtualServiceSSLConfig(virtualService *v1.VirtualService, secrets secretwatcher.SecretMap) error {
	if virtualService.SslConfig == nil || virtualService.SslConfig.SslSecrets == nil {
		return nil
	}
	_, _, _, err := getSslSecrets(virtualService.SslConfig.SslSecrets.(*v1.SSLConfig_SecretRef).SecretRef, secrets)
	return err
}

const (
	deprecatedSslCertificateChainKey = "ca_chain"
	deprecatedSslPrivateKeyKey       = "private_key"
)

func getSslSecrets(ref string, secrets secretwatcher.SecretMap) (string, string, string, error) {
	sslSecrets, ok := secrets[ref]
	if !ok {
		return "", "","", errors.Errorf("ssl secret not found for ref %v", ref)
	}
	certChain, ok := sslSecrets.Data[defaultv1.SslCertificateChainKey]
	if !ok {
		certChain, ok = sslSecrets.Data[deprecatedSslCertificateChainKey]
		if !ok {
			return "", "", "", errors.Errorf("neither %v nor %v key not found in ssl secrets", defaultv1.SslCertificateChainKey, deprecatedSslCertificateChainKey)
		}
	}

	privateKey, ok := sslSecrets.Data[defaultv1.SslPrivateKeyKey]
	if !ok {
		privateKey, ok = sslSecrets.Data[deprecatedSslPrivateKeyKey]
		if !ok {
			return "", "", "", errors.Errorf("neither %v nor %v key not found in ssl secrets", defaultv1.SslPrivateKeyKey, deprecatedSslPrivateKeyKey)
		}
	}

	rootCa := sslSecrets.Data[defaultv1.SslRootCaKey]
	return certChain, privateKey, rootCa, nil
}
