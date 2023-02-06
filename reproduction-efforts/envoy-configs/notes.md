# empty-gloo vs. customer-case
1. I found that a new http_filter was added to `"listener-::-8080"`
```
{"name": "envoy.filters.http.dynamic_forward_proxy",
"typed_config": {
    "@type": "type.googleapis.com/envoy.extensions.filters.http.dynamic_forward_proxy.v3.FilterConfig",
    "dns_cache_config": {
        "name": "solo_io_generated_dfp:0",
        "dns_lookup_family": "V4_PREFERRED"
    }
}}
```

2. I found that `dynamic_active_listeners` was actually populated (with a bunch of defaults).
3. I found that a `solo_io_generated_dfp:9830940034953162036` cluster was generated
```
{"version_info": "5527355940560430529",
"cluster": {
    "@type": "type.googleapis.com/envoy.config.cluster.v3.Cluster",
    "name": "solo_io_generated_dfp:9830940034953162036",
    "connect_timeout": "5s",
    "lb_policy": "CLUSTER_PROVIDED",
    "cluster_type": {
        "name": "envoy.clusters.dynamic_forward_proxy",
        "typed_config": {
            "@type": "type.googleapis.com/envoy.extensions.clusters.dynamic_forward_proxy.v3.ClusterConfig",
            "dns_cache_config": {
                "name": "solo_io_generated_dfp:0",
                "dns_lookup_family": "V4_PREFERRED"
            }
        }
    }
},
"last_updated": "2023-01-31T15:10:30.371Z"}
```
4. I found that a host rewrite header was added to `listener-::-8080-routes`
```
"envoy.filters.http.dynamic_forward_proxy": {	
    "@type": "type.googleapis.com/envoy.extensions.filters.http.dynamic_forward_proxy.v3.PerRouteConfig",	
    "host_rewrite_header": "x-destination"	
},
```

# empty-gloo vs. add-dfp-to-gateway-proxy-ssl
compared to above, 1, 2, and 4 exist.    3 is missing.

# customer-case vs. proposed-fix
Changes:
1. added `sslConfig` to static virtual service
```yaml
  sslConfig:
    secretRef:
      name: gateway-tls
      namespace: gloo-system
```
2. set `dynamicForwardProxy` on `gateway-proxy-ssl`, rather than on `gateway-proxy`
```yaml
  httpGateway:
    options:
      dynamicForwardProxy: {}
```
Results:
1. `listener-::-8080` was removed/draining, and replaced with `listener-::-8443`
    * confirmed that the `dynamic_forward_proxy` `http_filter` was there
2. 