#![deny(clippy::unwrap_used, clippy::expect_used)]

//! Example filter for kgateway Envoy dynamic module.
//!
//! This crate is a reference skeleton showing the required public surface for a filter.
//! It is included in the workspace but is NOT registered in `module-init/src/lib.rs`,
//! so it has no effect on the built `librust_module.so`.
//!
//! To create a real filter based on this example, see `../../../../docs/guides/adding-a-filter.md`.

use envoy_proxy_dynamic_modules_rust_sdk::*;

/// Filter-level configuration, created once per `DynamicModuleFilter` entry in the Envoy config.
///
/// Must be `pub` so `module-init` can call `FilterConfig::new` by name.
pub struct FilterConfig {
    // Add your filter-wide config fields here.
}

impl FilterConfig {
    /// Parse and validate the filter config string supplied in the Envoy config.
    ///
    /// Return `None` to reject the config; Envoy will refuse to load the filter.
    pub fn new(_config: &str) -> Option<Self> {
        Some(Self {})
    }
}

impl<EHF: EnvoyHttpFilter> HttpFilterConfig<EHF> for FilterConfig {
    fn new_http_filter(&self, _envoy: &mut EHF) -> Box<dyn HttpFilter<EHF>> {
        Box::new(Filter {})
    }
}

/// Per-route configuration override.
///
/// Must be `pub` so `module-init` can call `PerRouteConfig::new` by name.
/// Remove this struct entirely if you don't need per-route overrides.
pub struct PerRouteConfig {}

impl PerRouteConfig {
    pub fn new(_config: &str) -> Option<Self> {
        Some(Self {})
    }
}

/// Per-request filter state. Private — only accessed through the `HttpFilter` trait.
struct Filter {}

impl<EHF: EnvoyHttpFilter> HttpFilter<EHF> for Filter {
    fn on_request_headers(
        &mut self,
        _http_filter: &mut EHF,
        _end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_request_headers_status {
        abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue
    }

    fn on_response_headers(
        &mut self,
        _http_filter: &mut EHF,
        _end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_response_headers_status {
        abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::Continue
    }
}

#[cfg(test)]
mod tests;
