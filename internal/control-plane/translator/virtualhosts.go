package translator

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/internal/control-plane/reporter"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

// VirtualHosts

func (t *Translator) computeVirtualHosts(role *v1.Role,
	cfg *v1.Config,
	erroredUpstreams map[string]bool,
	secrets secretwatcher.SecretMap) ([]envoyroute.VirtualHost, []reporter.ConfigObjectReport) {
	var (
		reports      []reporter.ConfigObjectReport
		virtualHosts []envoyroute.VirtualHost

		// this applies to the whole role, not an individual virtual service
		roleErr error
	)

	// check for bad domains, then add those errors to the vService error list
	vServicesWithBadDomains := findVirtualServicesWithConflictingDomains(cfg.VirtualServices)

	for _, virtualService := range cfg.VirtualServices {
		roleErr = vServicesWithBadDomains[virtualService.Name]

		envoyVirtualHost, err := t.computeVirtualHost(cfg.Upstreams, virtualService, erroredUpstreams, secrets)
		if roleErr != nil {
			// report the role err on the virtualservice too
			// TODO: find a way to connect errors from roles to the virtualservice
			// the virtualservice might be valid in other roles, this will force all roles to exclude
			// this virtualservice
			err = multierror.Append(err, roleErr)
		}
		reports = append(reports, createReport(virtualService, err))
		// don't append errored virtual services to the success list
		if err != nil {
			continue
		}
		if virtualService.SslConfig != nil && virtualService.SslConfig.SecretRef != "" {
			// TODO: allow user to specify require ALL tls or just external
			envoyVirtualHost.RequireTls = envoyroute.VirtualHost_ALL
		}
		virtualHosts = append(virtualHosts, envoyVirtualHost)
	}

	// add report for the role
	reports = append(reports, createReport(role, roleErr))

	return virtualHosts, reports
}

// adds errors to report if virtualservice domains are not unique
func findVirtualServicesWithConflictingDomains(virtualServices []*v1.VirtualService) map[string]error {
	domainsToVirtualServices := make(map[string][]string) // this shouldbe a 1-1 mapping
	// if len(domainsToVirtualServices[domain]) > 1, error
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
	erroredVServices := make(map[string]error)
	// see if we found any conflicts, if so, write reports
	for domain, vServices := range domainsToVirtualServices {
		if len(vServices) > 1 {
			for _, name := range vServices {
				erroredVServices[name] = multierror.Append(erroredVServices[name], errors.Errorf("domain %v is "+
					"shared by the following virtual services: %v", domain, vServices))
			}
		}
	}
	return erroredVServices
}

func (t *Translator) computeVirtualHost(upstreams []*v1.Upstream,
	virtualService *v1.VirtualService,
	erroredUpstreams map[string]bool,
	secrets secretwatcher.SecretMap) (envoyroute.VirtualHost, error) {
	var envoyRoutes []envoyroute.Route
	var vServiceErrors error
	for _, route := range virtualService.Routes {
		if err := validateRouteDestinations(upstreams, route, erroredUpstreams); err != nil {
			vServiceErrors = multierror.Append(vServiceErrors, err)
		}
		out := envoyroute.Route{}
		for _, plug := range t.plugins {
			routePlugin, ok := plug.(plugins.RoutePlugin)
			if !ok {
				continue
			}
			params := &plugins.RoutePluginParams{
				Upstreams: upstreams,
			}
			if err := routePlugin.ProcessRoute(params, route, &out); err != nil {
				vServiceErrors = multierror.Append(vServiceErrors, err)
			}
		}
		envoyRoutes = append(envoyRoutes, out)
	}

	// validate ssl config if the host specifies one
	if err := validateVirtualServiceSSLConfig(virtualService, secrets); err != nil {
		vServiceErrors = multierror.Append(vServiceErrors, err)
	}

	domains := virtualService.Domains
	if len(domains) == 0 || (len(domains) == 1 && domains[0] == "") {
		domains = []string{"*"}
	}

	// TODO: handle default virtualservice
	// TODO: handle ssl
	return envoyroute.VirtualHost{
		Name:    virtualHostName(virtualService.Name),
		Domains: domains,
		Routes:  envoyRoutes,
	}, vServiceErrors
}

func validateRouteDestinations(upstreams []*v1.Upstream, route *v1.Route, erroredUpstreams map[string]bool) error {
	// collect existing upstreams/functions for matching
	upstreamsAndTheirFunctions := make(map[string][]string)

	for _, upstream := range upstreams {
		// don't consider errored upstreams to be valid destinations
		if erroredUpstreams[upstream.Name] {
			log.Debugf("upstream %v had errors, it will not be a considered destination", upstream.Name)
			continue
		}
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

func getErroredUpstreams(clusterReports []reporter.ConfigObjectReport) map[string]bool {
	erroredUpstreams := make(map[string]bool)
	for _, report := range clusterReports {
		upstream, ok := report.CfgObject.(*v1.Upstream)
		if !ok {
			continue
		}
		if report.Err != nil {
			erroredUpstreams[upstream.Name] = true
		}
	}
	return erroredUpstreams
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
	if virtualService.SslConfig == nil || virtualService.SslConfig.SecretRef == "" {
		return nil
	}
	_, _, err := getSslSecrets(virtualService.SslConfig.SecretRef, secrets)
	return err
}

func getSslSecrets(ref string, secrets secretwatcher.SecretMap) (string, string, error) {
	sslSecrets, ok := secrets[ref]
	if !ok {
		return "", "", errors.Errorf("ssl secret not found for ref %v", ref)
	}
	certChain, ok := sslSecrets.Data[sslCertificateChainKey]
	if !ok {
		certChain, ok = sslSecrets.Data[deprecatedSslCertificateChainKey]
		if !ok {
			return "", "", errors.Errorf("neither %v nor %v key not found in ssl secrets", sslCertificateChainKey, deprecatedSslCertificateChainKey)
		}
	}

	privateKey, ok := sslSecrets.Data[sslPrivateKeyKey]
	if !ok {
		privateKey, ok = sslSecrets.Data[deprecatedSslPrivateKeyKey]
		if !ok {
			return "", "", errors.Errorf("neither %v nor %v key not found in ssl secrets", sslPrivateKeyKey, deprecatedSslPrivateKeyKey)
		}
	}
	return certChain, privateKey, nil
}
