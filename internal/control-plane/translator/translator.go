package translator

import (
	"fmt"
	"sort"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyendpoints "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"

	"github.com/hashicorp/go-multierror"
	"github.com/mitchellh/hashstructure"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/internal/control-plane/bootstrap"
	"github.com/solo-io/gloo/internal/control-plane/filewatcher"
	"github.com/solo-io/gloo/internal/control-plane/reporter"
	"github.com/solo-io/gloo/internal/control-plane/snapshot"
	"github.com/solo-io/gloo/internal/control-plane/translator/defaults"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/matcher"
	"github.com/solo-io/gloo/pkg/coreplugins/route-extensions"
	"github.com/solo-io/gloo/pkg/coreplugins/service"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

const (
	sslRdsName      = "gloo-rds-https"
	sslListenerName = "listener-" + sslRdsName

	noSslRdsName      = "gloo-rds-http"
	noSslListenerName = "listener-" + noSslRdsName

	connMgrFilter = "envoy.http_connection_manager"
	routerFilter  = "envoy.router"
)

type Translator struct {
	plugins []plugins.TranslatorPlugin
	config  bootstrap.IngressOptions
}

// all built-in plugins should go here
var corePlugins = []plugins.TranslatorPlugin{
	&matcher.Plugin{},
	&extensions.Plugin{},
	&service.Plugin{},
}

func NewTranslator(opts bootstrap.IngressOptions, translatorPlugins []plugins.TranslatorPlugin) *Translator {
	translatorPlugins = append(corePlugins, translatorPlugins...)
	// special routing must be done for upstream plugins that support functions
	var functionPlugins []plugins.FunctionPlugin
	for _, plug := range translatorPlugins {
		if functionPlugin, ok := plug.(plugins.FunctionPlugin); ok {
			functionPlugins = append(functionPlugins, functionPlugin)
		}
	}

	// the route initializer plugin intializes route actions and metadata
	// including cluster weights for upstream and function destinations
	routeInitializer := newRouteInitializerPlugin()

	// the functional upstream plugins call ParseFunctionSpec on each function plugin
	// and adds the function spec to the cluster metadata
	// the functional upstream processor should be inserted at the end of the plugin chain
	// to ensure ProcessUpstream() is called before ParseFunctionSpec for each upstream
	functionalUpstreamProcessor := newFunctionalPluginProcessor(functionPlugins)

	// order matters here
	translatorPlugins = append([]plugins.TranslatorPlugin{routeInitializer}, translatorPlugins...)
	translatorPlugins = append(translatorPlugins, functionalUpstreamProcessor)

	return &Translator{
		plugins: translatorPlugins,
		config:  opts,
	}
}

type pluginDependencies struct {
	Secrets secretwatcher.SecretMap
	Files   filewatcher.Files
}

func (t *Translator) Translate(role *v1.Role, inputs *snapshot.Cache) (*envoycache.Snapshot, []reporter.ConfigObjectReport, error) {
	cfg := inputs.Cfg
	dependencies := &pluginDependencies{Secrets: inputs.Secrets, Files: inputs.Files}
	secrets := inputs.Secrets
	endpoints := inputs.Endpoints

	log.Printf("Translation loop starting")
	// endpoints
	clusterLoadAssignments := computeClusterEndpoints(cfg.Upstreams, endpoints)

	// clusters
	clusters, upstreamReports := t.computeClusters(cfg, dependencies, endpoints)

	// mark errored upstreams; routes that point to them are considered invalid
	errored := getErroredUpstreams(upstreamReports)

	// envoy virtual hosts
	sslVirtualHosts, noSslVirtualHosts, virtualServiceReports := t.computeVirtualHosts(role, cfg, errored, secrets)

	noSslRouteConfig := &envoyapi.RouteConfiguration{
		Name:         noSslRdsName,
		VirtualHosts: noSslVirtualHosts,
	}

	// create the base http filters which both listeners will implement
	httpFilters := t.createHttpFilters()

	// filters
	// they are basically the same, but have different rds names

	// http filters
	noSslFilters, err := t.constructFilters(noSslRouteConfig.Name, httpFilters)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "constructing http filter chain %v", noSslListenerName)
	}
	noSslListener := t.constructHttpListener(noSslListenerName, t.config.Port, noSslFilters)

	// https filters
	sslRouteConfig := &envoyapi.RouteConfiguration{
		Name:         sslRdsName,
		VirtualHosts: sslVirtualHosts,
	}

	sslFilters, err := t.constructFilters(sslRouteConfig.Name, httpFilters)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "constructing https filter chain %v", sslListenerName)
	}

	// finally, the listeners
	httpsListener, err := t.constructHttpsListener(sslListenerName,
		t.config.SecurePort,
		sslFilters,
		cfg.VirtualServices,
		virtualServiceReports,
		secrets)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "constructing https listener %v", sslListenerName)
	}

	// proto-ify everything
	var endpointsProto []envoycache.Resource
	for _, cla := range clusterLoadAssignments {
		endpointsProto = append(endpointsProto, cla)
	}

	var clustersProto []envoycache.Resource
	for _, cluster := range clusters {
		clustersProto = append(clustersProto, cluster)
	}

	var listenersProto, routesProto []envoycache.Resource

	// only add http listener and route config if we have no ssl vServices
	if len(noSslVirtualHosts) > 0 && len(noSslListener.FilterChains) > 0 {
		listenersProto = append(listenersProto, noSslListener)
		routesProto = append(routesProto, noSslRouteConfig)
	}

	// only add https listener and route config if we have ssl vServices
	if len(sslVirtualHosts) > 0 && len(httpsListener.FilterChains) > 0 {
		listenersProto = append(listenersProto, httpsListener)
		routesProto = append(routesProto, sslRouteConfig)
	}

	// construct version
	// TODO: investigate whether we need a more sophisticated versionining algorithm
	version, err := hashstructure.Hash([][]envoycache.Resource{
		endpointsProto,
		clustersProto,
		routesProto,
		listenersProto,
	}, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "constructing version hash for envoy snapshot components")
	}
	// construct snapshot
	xdsSnapshot := envoycache.NewSnapshot(fmt.Sprintf("%v", version), endpointsProto, clustersProto, routesProto, listenersProto)

	// aggregate reports
	reports := append(upstreamReports, virtualServiceReports...)

	return &xdsSnapshot, reports, nil
}

// Endpoints

func computeClusterEndpoints(upstreams []*v1.Upstream, endpoints endpointdiscovery.EndpointGroups) []*envoyapi.ClusterLoadAssignment {
	var clusterEndpointAssignments []*envoyapi.ClusterLoadAssignment
	for _, upstream := range upstreams {
		// if there is an endpoint group for this upstream,
		// it's using eds and we need to create a load assignment for it
		if endpointGroup, ok := endpoints[upstream.Name]; ok {
			loadAssignment := loadAssignmentForCluster(upstream.Name, endpointGroup)
			clusterEndpointAssignments = append(clusterEndpointAssignments, loadAssignment)
		}
	}
	return clusterEndpointAssignments
}

func loadAssignmentForCluster(clusterName string, addresses []endpointdiscovery.Endpoint) *envoyapi.ClusterLoadAssignment {
	var endpoints []envoyendpoints.LbEndpoint
	for _, addr := range addresses {
		lbEndpoint := envoyendpoints.LbEndpoint{
			Endpoint: &envoyendpoints.Endpoint{
				Address: &envoycore.Address{
					Address: &envoycore.Address_SocketAddress{
						SocketAddress: &envoycore.SocketAddress{
							Protocol: envoycore.TCP,
							Address:  addr.Address,
							PortSpecifier: &envoycore.SocketAddress_PortValue{
								PortValue: uint32(addr.Port),
							},
						},
					},
				},
			},
		}
		endpoints = append(endpoints, lbEndpoint)
	}

	return &envoyapi.ClusterLoadAssignment{
		ClusterName: clusterName,
		Endpoints: []envoyendpoints.LocalityLbEndpoints{{
			LbEndpoints: endpoints,
		}},
	}
}

// Clusters

func (t *Translator) computeClusters(cfg *v1.Config, dependencies *pluginDependencies, endpoints endpointdiscovery.EndpointGroups) ([]*envoyapi.Cluster, []reporter.ConfigObjectReport) {
	var (
		reports  []reporter.ConfigObjectReport
		clusters []*envoyapi.Cluster
	)
	for _, upstream := range cfg.Upstreams {
		_, edsCluster := endpoints[upstream.Name]
		cluster, err := t.computeCluster(cfg, dependencies, upstream, edsCluster)
		// only append valid clusters
		if err == nil {
			clusters = append(clusters, cluster)
		}
		reports = append(reports, createReport(upstream, err))
	}
	return clusters, reports
}

func (t *Translator) computeCluster(cfg *v1.Config, dependencies *pluginDependencies, upstream *v1.Upstream, edsCluster bool) (*envoyapi.Cluster, error) {
	out := &envoyapi.Cluster{
		Name:     upstream.Name,
		Metadata: new(envoycore.Metadata),
	}
	if edsCluster {
		out.Type = envoyapi.Cluster_EDS
	}

	timeout := upstream.ConnectionTimeout
	if timeout == 0 {
		timeout = defaults.ClusterConnectionTimeout
	}
	out.ConnectTimeout = timeout

	var upstreamErrors error
	for _, plug := range t.plugins {
		upstreamPlugin, ok := plug.(plugins.UpstreamPlugin)
		if !ok {
			continue
		}
		params := &plugins.UpstreamPluginParams{
			EnvoyNameForUpstream: clusterName,
		}
		deps := dependenciesForPlugin(cfg, upstreamPlugin, dependencies)
		if deps != nil {
			params.Secrets = deps.Secrets
			params.Files = deps.Files
		}

		if err := upstreamPlugin.ProcessUpstream(params, upstream, out); err != nil {
			upstreamErrors = multierror.Append(upstreamErrors, err)
		}
	}
	if err := validateCluster(out); err != nil {
		upstreamErrors = multierror.Append(upstreamErrors, err)
	}
	return out, upstreamErrors
}

// TODO: add more validation here
func validateCluster(c *envoyapi.Cluster) error {
	if c.Type == envoyapi.Cluster_STATIC || c.Type == envoyapi.Cluster_STRICT_DNS || c.Type == envoyapi.Cluster_LOGICAL_DNS {
		if len(c.Hosts) < 1 {
			return errors.Errorf("cluster type %v specified but hosts were empty", c.Type.String())
		}
	}
	return nil
}

func dependenciesForPlugin(cfg *v1.Config, plug plugins.TranslatorPlugin, dependencies *pluginDependencies) *pluginDependencies {
	dependencyRefs := plug.GetDependencies(cfg)
	if dependencyRefs == nil {
		return nil
	}
	pluginDeps := &pluginDependencies{
		Secrets: make(secretwatcher.SecretMap),
		Files:   make(filewatcher.Files),
	}
	for _, ref := range dependencyRefs.SecretRefs {
		item, ok := dependencies.Secrets[ref]
		if ok {
			pluginDeps.Secrets[ref] = item
		}
	}
	for _, ref := range dependencyRefs.FileRefs {
		item, ok := dependencies.Files[ref]
		if ok {
			pluginDeps.Files[ref] = item
		}
	}
	return pluginDeps
}

// VirtualServices

func (t *Translator) computeVirtualHosts(role *v1.Role,
	cfg *v1.Config,
	erroredUpstreams map[string]bool,
	secrets secretwatcher.SecretMap) ([]envoyroute.VirtualHost, []envoyroute.VirtualHost, []reporter.ConfigObjectReport) {
	var (
		reports           []reporter.ConfigObjectReport
		sslVirtualHosts   []envoyroute.VirtualHost
		noSslVirtualHosts []envoyroute.VirtualHost

		// this applies to the whole role, not an individual virtual service
		roleErr error
	)

	// check for bad domains, then add those errors to the vService error list
	vServicesWithBadDomains := findVirtualServicesWithConflictingDomains(cfg.VirtualServices)

	for _, virtualService := range cfg.VirtualServices {
		if domainErr, invalidVService := vServicesWithBadDomains[virtualService.Name]; invalidVService {
			roleErr = multierror.Append(roleErr, domainErr)
		}

		envoyVirtualHost, err := t.computeVirtualHost(cfg.Upstreams, virtualService, erroredUpstreams, secrets)
		reports = append(reports, createReport(virtualService, err))
		// don't append errored virtual services
		if err != nil {
			continue
		}
		if virtualService.SslConfig != nil && virtualService.SslConfig.SecretRef != "" {
			// TODO: allow user to specify require ALL tls or just external
			envoyVirtualHost.RequireTls = envoyroute.VirtualHost_ALL
			sslVirtualHosts = append(sslVirtualHosts, envoyVirtualHost)
		} else {
			noSslVirtualHosts = append(noSslVirtualHosts, envoyVirtualHost)
		}
	}

	// add report for the role
	reports = append(reports, createReport(role, roleErr))

	return sslVirtualHosts, noSslVirtualHosts, reports
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
		return "", "", errors.Errorf("key %v not found in ssl secrets", sslCertificateChainKey)
	}
	privateKey, ok := sslSecrets.Data[sslPrivateKeyKey]
	if !ok {
		return "", "", errors.Errorf("key %v not found in ssl secrets", sslPrivateKeyKey)
	}
	return certChain, privateKey, nil
}

// Listener

type stagedFilter struct {
	filter *envoyhttp.HttpFilter
	stage  plugins.Stage
}

func (t *Translator) constructHttpListener(name string, port uint32, filters []envoylistener.Filter) *envoyapi.Listener {
	return &envoyapi.Listener{
		Name: name,
		Address: envoycore.Address{
			Address: &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Protocol: envoycore.TCP,
					Address:  t.config.BindAddress,
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: port,
					},
					Ipv4Compat: true,
				},
			},
		},
		FilterChains: []envoylistener.FilterChain{{
			Filters: filters,
		}},
	}
}

const (
	sslCertificateChainKey = "ca_chain"
	sslPrivateKeyKey       = "private_key"
)

func (t *Translator) constructHttpsListener(name string,
	port uint32,
	filters []envoylistener.Filter,
	virtualServices []*v1.VirtualService,
	virtualServiceReports []reporter.ConfigObjectReport,
	secrets secretwatcher.SecretMap) (*envoyapi.Listener, error) {

	// create the base filter chain
	// we will copy the filter chain for each virtualservice that specifies an ssl config
	var filterChains []envoylistener.FilterChain
	for _, vService := range virtualServices {
		if vService.SslConfig == nil || vService.SslConfig.SecretRef == "" {
			continue
		}
		ref := vService.SslConfig.SecretRef
		certChain, privateKey, err := getSslSecrets(ref, secrets)
		if err != nil {
			log.Warnf("skipping ssl vService with invalid secrets: %v", vService.Name)
			continue
		}
		filterChain := newSslFilterChain(certChain, privateKey, filters)
		filterChains = append(filterChains, filterChain)
	}

	return &envoyapi.Listener{
		Name: name,
		Address: envoycore.Address{
			Address: &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Protocol: envoycore.TCP,
					Address:  t.config.BindAddress,
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: port,
					},
					Ipv4Compat: true,
				},
			},
		},
		FilterChains: filterChains,
	}, nil
}

func newSslFilterChain(certChain, privateKey string, filters []envoylistener.Filter) envoylistener.FilterChain {
	return envoylistener.FilterChain{
		Filters: filters,
		TlsContext: &envoyauth.DownstreamTlsContext{
			CommonTlsContext: &envoyauth.CommonTlsContext{
				// default params
				TlsParams: &envoyauth.TlsParameters{},
				// TODO: configure client certificates
				TlsCertificates: []*envoyauth.TlsCertificate{
					{
						CertificateChain: &envoycore.DataSource{
							Specifier: &envoycore.DataSource_InlineString{
								InlineString: certChain,
							},
						},
						PrivateKey: &envoycore.DataSource{
							Specifier: &envoycore.DataSource_InlineString{
								InlineString: privateKey,
							},
						},
					},
				},
			},
		},
	}
}

func (t *Translator) createHttpFilters() []*envoyhttp.HttpFilter {
	var filtersByStage []stagedFilter
	for _, plug := range t.plugins {
		filterPlugin, ok := plug.(plugins.FilterPlugin)
		if !ok {
			continue
		}
		params := &plugins.FilterPluginParams{}
		stagedFilters := filterPlugin.HttpFilters(params)
		for _, httpFilter := range stagedFilters {
			if httpFilter.HttpFilter == nil {
				log.Warnf("plugin implements HttpFilters() but returned nil")
				continue
			}
			filtersByStage = append(filtersByStage, stagedFilter{
				filter: httpFilter.HttpFilter,
				stage:  httpFilter.Stage,
			})
		}
	}

	// sort filters by stage
	httpFilters := sortFilters(filtersByStage)
	httpFilters = append(httpFilters, &envoyhttp.HttpFilter{Name: routerFilter})
	return httpFilters
}

func (t *Translator) constructFilters(routeConfigName string, httpFilters []*envoyhttp.HttpFilter) ([]envoylistener.Filter, error) {
	httpConnMgr := &envoyhttp.HttpConnectionManager{
		CodecType:  envoyhttp.AUTO,
		StatPrefix: "http",
		RouteSpecifier: &envoyhttp.HttpConnectionManager_Rds{
			Rds: &envoyhttp.Rds{
				ConfigSource: envoycore.ConfigSource{
					ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{
						Ads: &envoycore.AggregatedConfigSource{},
					},
				},
				RouteConfigName: routeConfigName,
			},
		},
		HttpFilters: httpFilters,
	}

	httpConnMgrCfg, err := envoyutil.MessageToStruct(httpConnMgr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert proto message to struct")
	}
	return []envoylistener.Filter{
		{
			Name:   connMgrFilter,
			Config: httpConnMgrCfg,
		},
	}, nil
}

func sortFilters(filters []stagedFilter) []*envoyhttp.HttpFilter {
	// sort them first by stage, then by name.
	less := func(i, j int) bool {
		filteri := filters[i]
		filterj := filters[j]
		if filteri.stage != filterj.stage {
			return filteri.stage < filterj.stage
		}
		return filteri.filter.Name < filterj.filter.Name
	}
	sort.SliceStable(filters, less)

	var sortedFilters []*envoyhttp.HttpFilter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.filter)
	}

	return sortedFilters
}

// for future-proofing possible safety issues with bad upstream names
func clusterName(upstreamName string) string {
	return upstreamName
}

// for future-proofing possible safety issues with bad virtualservice names
func virtualHostName(virtualServiceName string) string {
	return virtualServiceName
}

func createReport(cfgObject v1.ConfigObject, err error) reporter.ConfigObjectReport {
	return reporter.ConfigObjectReport{
		CfgObject: cfgObject,
		Err:       err,
	}
}
