use envoy_proxy_dynamic_modules_rust_sdk::*;

// ALL FILTERS HERE
mod http_simple_mutations;

declare_init_functions!(init, new_http_filter_config_fn);

/// This implements the [`envoy_proxy_dynamic_modules_rust_sdk::ProgramInitFunction`].
///
/// This is called exactly once when the module is loaded. It can be used to
/// initialize global state as well as check the runtime environment to ensure that
/// the module is running in a supported environment.
///
/// Returning `false` will cause Envoy to reject the config hence the
/// filter will not be loaded.
fn init() -> bool {
    true
}

/// This implements the [`envoy_proxy_dynamic_modules_rust_sdk::NewHttpFilterConfigFunction`].
///
/// This is the entrypoint every time a new HTTP filter is created via the DynamicModuleFilter config.
///
/// Each argument matches the corresponding argument in the Envoy config here:
/// https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/dynamic_modules/v3/dynamic_modules.proto#envoy-v3-api-msg-extensions-dynamic-modules-v3-dynamicmoduleconfig
///
/// Returns None if the filter name or config is determined to be invalid by each filter's `new` function.
fn new_http_filter_config_fn<EC: EnvoyHttpFilterConfig, EHF: EnvoyHttpFilter>(
    _envoy_filter_config: &mut EC,
    filter_name: &str,
    filter_config: &[u8],
) -> Option<Box<dyn HttpFilterConfig<EC, EHF>>> {
    let filter_config = match std::str::from_utf8(filter_config) {
        Ok(config) => config,
        Err(_) => {
            eprintln!("Invalid UTF-8 in filter configuration");
            return None;
        }
    };
    match filter_name {
        "http_simple_mutations" => http_simple_mutations::FilterConfig::new(filter_config)
            .map(|config| Box::new(config) as Box<dyn HttpFilterConfig<EC, EHF>>),
        _ => panic!(
            "Unknown filter name: {}, known filters are {}",
            filter_name, "http_simple_mutations"
        ),
    }
}
