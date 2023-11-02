/*
Package enterprise_warning creates the EnterpriseWarning plugin.
EnterpriseWarning plugin is responsible for identifying Enterprise config that a
non-Enterprise install has configured, and warning on it.
NOTE: This is probably not the ideal solution, and it would be nice to find something more robust
However, it follows a pattern used in our SyncerExtensions (for ratelimit and extauth)
so its good to be consistent and good enough for now.
*/
package enterprise_warning

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var (
	_ plugins.Plugin            = new(plugin)
	_ plugins.HttpFilterPlugin  = new(plugin)
	_ plugins.VirtualHostPlugin = new(plugin)
	_ plugins.RoutePlugin       = new(plugin)
)

// A series of names to be emitted in errors
// when an enterprise-only extension is used.
const (
	ExtensionName = "enterprise_warning"

	AdvancedHttpExtensionName          = "advanced_http"
	CachingExtensionName               = "caching"
	DlpExtensionName                   = "dlp"
	FailoverExtensionName              = "failover"
	JwtExtensionName                   = "jwt"
	LeftmostXffAddressExtensionName    = "leftmost_xff_address"
	ProxyLatencyExtensionName          = "proxy_latency"
	RbacExtensionName                  = "rbac"
	SanitizeClusterHeaderExtensionName = "sanitize_cluster_header"
	WafExtensionName                   = "waf"
	WasmExtensionName                  = "wasm"
	Aws                                = "aws"
	ExtProcExtensionName               = "extproc"
	TapFilterExtensionName             = "tap"
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
}

func GetErrorForEnterpriseOnlyExtensions(extensionNames []string) error {
	if len(extensionNames) == 0 {
		return nil
	}
	return errors.Errorf("Could not load configuration for the following Enterprise features: %v", extensionNames)
}

func (p *plugin) ProcessVirtualHost(
	_ plugins.VirtualHostParams,
	in *v1.VirtualHost,
	_ *envoy_config_route_v3.VirtualHost,
) error {
	var enterpriseExtensions []string

	if isJwtConfiguredOnVirtualHost(in) {
		enterpriseExtensions = append(enterpriseExtensions, JwtExtensionName)
	}

	if isRbacConfiguredOnVirtualHost(in) {
		enterpriseExtensions = append(enterpriseExtensions, RbacExtensionName)
	}

	if isWafConfiguredOnVirtualHost(in) {
		enterpriseExtensions = append(enterpriseExtensions, WafExtensionName)
	}

	if isExtProcConfiguredOnVirtualHost(in) {
		enterpriseExtensions = append(enterpriseExtensions, ExtProcExtensionName)
	}

	return GetErrorForEnterpriseOnlyExtensions(enterpriseExtensions)
}

func (p *plugin) ProcessRoute(_ plugins.RouteParams, in *v1.Route, _ *envoy_config_route_v3.Route) error {
	var enterpriseExtensions []string

	if isJwtConfiguredOnRoute(in) {
		enterpriseExtensions = append(enterpriseExtensions, JwtExtensionName)
	}

	if isRbacConfiguredOnRoute(in) {
		enterpriseExtensions = append(enterpriseExtensions, RbacExtensionName)
	}

	if isWafConfiguredOnRoute(in) {
		enterpriseExtensions = append(enterpriseExtensions, WafExtensionName)
	}

	if isEnterpriseAWSConfiguredOnRoute(in) {
		enterpriseExtensions = append(enterpriseExtensions, Aws)
	}

	if isExtProcConfiguredOnRoute(in) {
		enterpriseExtensions = append(enterpriseExtensions, ExtProcExtensionName)
	}

	return GetErrorForEnterpriseOnlyExtensions(enterpriseExtensions)
}

func (p *plugin) ProcessUpstream(_ plugins.Params, in *v1.Upstream, _ *envoy_config_cluster_v3.Cluster) error {
	var enterpriseExtensions []string

	if isAdvancedHttpConfiguredOnUpstream(in) {
		enterpriseExtensions = append(enterpriseExtensions, AdvancedHttpExtensionName)
	}

	if isFailoverConfiguredOnUpstream(in) {
		enterpriseExtensions = append(enterpriseExtensions, FailoverExtensionName)
	}

	return GetErrorForEnterpriseOnlyExtensions(enterpriseExtensions)
}

func (p *plugin) HttpFilters(_ plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	var enterpriseExtensions []string

	if isCachingConfiguredOnListener(listener) {
		enterpriseExtensions = append(enterpriseExtensions, CachingExtensionName)
	}

	if isDlpConfiguredOnListener(listener) {
		enterpriseExtensions = append(enterpriseExtensions, DlpExtensionName)
	}

	if isLeftmostXffAddressConfiguredOnListener(listener) {
		enterpriseExtensions = append(enterpriseExtensions, LeftmostXffAddressExtensionName)
	}

	if isProxyLatencyConfiguredOnListener(listener) {
		enterpriseExtensions = append(enterpriseExtensions, ProxyLatencyExtensionName)
	}

	if isSanitizeClusterHeaderConfiguredOnListener(listener) {
		enterpriseExtensions = append(enterpriseExtensions, SanitizeClusterHeaderExtensionName)
	}

	if isWafConfiguredOnListener(listener) {
		enterpriseExtensions = append(enterpriseExtensions, WafExtensionName)
	}

	if isWasmConfiguredOnListener(listener) {
		enterpriseExtensions = append(enterpriseExtensions, WasmExtensionName)
	}

	if isExtProcConfiguredOnListener(listener) {
		enterpriseExtensions = append(enterpriseExtensions, ExtProcExtensionName)
	}

	if isTapConfiguredOnListener(listener) {
		enterpriseExtensions = append(enterpriseExtensions, TapFilterExtensionName)
	}

	return nil, GetErrorForEnterpriseOnlyExtensions(enterpriseExtensions)
}

// advanced_http
func isAdvancedHttpConfiguredOnUpstream(in *v1.Upstream) bool {
	for _, host := range in.GetStatic().GetHosts() {
		if host.GetHealthCheckConfig().GetPath() != "" {
			return true
		}
		if host.GetHealthCheckConfig().GetMethod() != "" {
			return true
		}
	}

	for _, hc := range in.GetHealthChecks() {
		if hc.GetHttpHealthCheck().GetResponseAssertions() != nil {
			return true
		}
	}

	return false
}

// caching
func isCachingConfiguredOnListener(in *v1.HttpListener) bool {
	return in.GetOptions().GetCaching() != nil
}

// dlp
func isDlpConfiguredOnListener(in *v1.HttpListener) bool {
	return in.GetOptions().GetDlp() != nil
}

// failover
func isFailoverConfiguredOnUpstream(in *v1.Upstream) bool {
	return in.GetFailover() != nil
}

// jwt
func isJwtConfiguredOnVirtualHost(in *v1.VirtualHost) bool {
	return in.GetOptions().GetJwtConfig() != nil
}

func isJwtConfiguredOnRoute(in *v1.Route) bool {
	return in.GetOptions().GetJwtConfig() != nil
}

// leftmost_xff_address
func isLeftmostXffAddressConfiguredOnListener(in *v1.HttpListener) bool {
	return in.GetOptions().GetLeftmostXffAddress() != nil
}

// proxy_latency
func isProxyLatencyConfiguredOnListener(in *v1.HttpListener) bool {
	return in.GetOptions().GetProxyLatency() != nil
}

// rbac
func isRbacConfiguredOnVirtualHost(in *v1.VirtualHost) bool {
	return in.GetOptions().GetRbac() != nil
}

func isRbacConfiguredOnRoute(in *v1.Route) bool {
	return in.GetOptions().GetRbac() != nil
}

// sanitize_cluster_header
func isSanitizeClusterHeaderConfiguredOnListener(in *v1.HttpListener) bool {
	return in.GetOptions().GetSanitizeClusterHeader() != nil
}

// tap
func isTapConfiguredOnListener(in *v1.HttpListener) bool {
	return in.GetOptions().GetTap() != nil
}

// waf
func isWafConfiguredOnVirtualHost(in *v1.VirtualHost) bool {
	return in.GetOptions().GetWaf() != nil
}

func isWafConfiguredOnRoute(in *v1.Route) bool {
	return in.GetOptions().GetWaf() != nil
}

func isWafConfiguredOnListener(in *v1.HttpListener) bool {
	return in.GetOptions().GetWaf() != nil
}

// wasm
func isWasmConfiguredOnListener(in *v1.HttpListener) bool {
	return in.GetOptions().GetWasm() != nil
}

// aws
func isEnterpriseAWSConfiguredOnRoute(in *v1.Route) bool {
	var awsDestinationSpecs []*aws.DestinationSpec

	if singleDestinationSpec := in.GetRouteAction().GetSingle().GetDestinationSpec().GetAws(); singleDestinationSpec != nil {
		awsDestinationSpecs = append(awsDestinationSpecs, singleDestinationSpec)
	}

	for _, weightedDestination := range in.GetRouteAction().GetMulti().GetDestinations() {
		if weightedDestinationSpec := weightedDestination.GetDestination().GetDestinationSpec().GetAws(); weightedDestinationSpec != nil {
			awsDestinationSpecs = append(awsDestinationSpecs, weightedDestinationSpec)
		}
	}

	for _, awsDestinationSpec := range awsDestinationSpecs {
		if awsDestinationSpec.GetWrapAsApiGateway() {
			// this is an enterprise only feature
			return true
		}
	}

	return false
}

// extproc
func isExtProcConfiguredOnVirtualHost(in *v1.VirtualHost) bool {
	return in.GetOptions().GetExtProc() != nil
}

func isExtProcConfiguredOnRoute(in *v1.Route) bool {
	return in.GetOptions().GetExtProc() != nil
}

func isExtProcConfiguredOnListener(in *v1.HttpListener) bool {
	return in.GetOptions().GetExtProc() != nil || in.GetOptions().GetDisableExtProc() != nil
}
