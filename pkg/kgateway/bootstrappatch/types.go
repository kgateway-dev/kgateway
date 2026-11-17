package bootstrappatch

// protojson resolves each `@type` in a bootstrap through Go's global type registry, so
// every type the generated bootstrap (pkg/kgateway/helm/envoy/templates/configmap.yaml)
// uses must be linked into the binary that parses it. This package links each of them
// itself rather than relying on other packages in the binary to.
// TestGeneratedBootstrapParses in pkg/kgateway/deployer fails when the template starts
// using a type that is not listed here.
import (
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dns/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/credential_injector/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/health_check/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/upstream_codec/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/proxy_protocol/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/http/injected_credentials/generic/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/network/dns_resolver/cares/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/resource_monitors/downstream_connections/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
)
