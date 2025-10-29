use envoy_proxy_dynamic_modules_rust_sdk::*;
use lazy_static::lazy_static;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[cfg(test)]
use mockall::*;

lazy_static! {
    static ref EMPTY_MAP: HashMap<String, String> = HashMap::new();
}
#[derive(Serialize, Deserialize, Clone)]
pub struct PerRouteConfig {
    #[serde(default)]
    request_headers_setter: Vec<(String, String)>,
    #[serde(default)]
    response_headers_setter: Vec<(String, String)>,
}

impl PerRouteConfig {
    pub fn new(config: &str) -> Option<Self> {
        let per_route_config: PerRouteConfig = match serde_json::from_str(config) {
            Ok(cfg) => cfg,
            Err(err) => {
                envoy_log_error!("Error parsing per route config: {config} {err}");
                return None;
            }
        };
        Some(per_route_config)
    }
}

#[derive(Serialize, Deserialize, Clone)]
pub struct FilterConfig {
    #[serde(default)]
    request_headers_setter: Vec<(String, String)>,
    #[serde(default)]
    response_headers_setter: Vec<(String, String)>,
}

impl FilterConfig {
    /// This is the constructor for the [`FilterConfig`].
    ///
    /// filter_config is the filter config from the Envoy config here:
    /// https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/dynamic_modules/v3/dynamic_modules.proto#envoy-v3-api-msg-extensions-dynamic-modules-v3-dynamicmoduleconfig
    pub fn new(filter_config: &str) -> Option<Self> {
        let filter_config: FilterConfig = match serde_json::from_str(filter_config) {
            // TODO(nfuden): Handle optional configuration entries more cleanly. Currently all values are required to be present
            Ok(cfg) => cfg,
            Err(err) => {
                // TODO(nfuden): Dont panic if there is incorrect configuration
                envoy_log_error!("Error parsing filter config: {filter_config} {err}");
                return None;
            }
        };
        Some(filter_config)
    }
}

impl<EHF: EnvoyHttpFilter> HttpFilterConfig<EHF> for FilterConfig {
    /// This is called for each new HTTP filter.
    fn new_http_filter(&mut self, _envoy: &mut EHF) -> Box<dyn HttpFilter<EHF>> {
        Box::new(Filter {
            filter_config: self.clone(),
            per_route_config: None,
            env: transformations::jinja::new_jinja_env(),
            request_headers_map: None,
        })
    }
}

pub struct Filter {
    filter_config: FilterConfig,
    per_route_config: Option<Box<PerRouteConfig>>,
    env: minijinja::Environment<'static>,
    request_headers_map: Option<HashMap<String, String>>,
}

impl Filter {
    fn set_per_route_config<EHF: EnvoyHttpFilter>(&mut self, envoy_filter: &mut EHF) {
        if self.per_route_config.is_none() {
            if let Some(per_route_config) = envoy_filter.get_most_specific_route_config().as_ref() {
                let per_route_config = match per_route_config.downcast_ref::<PerRouteConfig>() {
                    Some(cfg) => cfg,
                    None => {
                        envoy_log_error!(
                            "set_per_route_config: wrong per route config type: {:?}",
                            per_route_config
                        );
                        return;
                    }
                };
                self.per_route_config = Some(Box::new(per_route_config.clone()));
            }
        }
    }

    fn get_per_route_config(&self) -> Option<&PerRouteConfig> {
        self.per_route_config.as_deref()
    }

    fn create_headers_map(
        &self,
        headers: Vec<(EnvoyBuffer, EnvoyBuffer)>,
    ) -> HashMap<String, String> {
        let mut headers_map = HashMap::new();
        for (key, val) in headers {
            let Some(key) = std::str::from_utf8(key.as_slice()).ok() else {
                continue;
            };
            let value = std::str::from_utf8(val.as_slice()).unwrap().to_string();

            headers_map.insert(key.to_string(), value);
        }

        headers_map
    }

    // This function is used to populate the self.request_headers_map so we only ever
    // do it once while we might need the request headers in either on_request_headers() or
    // on_response_headers().
    fn populate_request_headers_map(&mut self, headers: Vec<(EnvoyBuffer, EnvoyBuffer)>) {
        if self.request_headers_map.is_none() {
            let mut request_headers_map = HashMap::new();
            for (key, val) in headers {
                let Some(key) = std::str::from_utf8(key.as_slice()).ok() else {
                    continue;
                };
                let value = std::str::from_utf8(val.as_slice()).unwrap().to_string();

                request_headers_map.insert(key.to_string(), value);
            }

            self.request_headers_map = Some(request_headers_map);
        }
    }

    fn get_request_headers_map(&self) -> &HashMap<String, String> {
        self.request_headers_map.as_ref().unwrap_or(&EMPTY_MAP)
    }

    fn transform_request_headers<EHF: EnvoyHttpFilter>(&self, envoy_filter: &mut EHF) {
        let setters = match self.get_per_route_config() {
            Some(config) => &config.request_headers_setter,
            None => &self.filter_config.request_headers_setter,
        };

        transformations::jinja::transform_request_headers(
            setters,
            &self.env,
            self.get_request_headers_map(),
            |key, value| envoy_filter.set_request_header(key, value),
        );
    }

    fn transform_response_headers<EHF: EnvoyHttpFilter>(&self, envoy_filter: &mut EHF) {
        let setters = match self.get_per_route_config() {
            Some(config) => &config.response_headers_setter,
            None => &self.filter_config.response_headers_setter,
        };

        // TODO(nfuden): find someone who knows rust to see if we really need this Hash map for serialization
        let response_headers_map = self.create_headers_map(envoy_filter.get_response_headers());

        transformations::jinja::transform_response_headers(
            setters,
            &self.env,
            self.get_request_headers_map(),
            &response_headers_map,
            |key, value| envoy_filter.set_response_header(key, value),
        );
    }
}

/// This implements the [`envoy_proxy_dynamic_modules_rust_sdk::HttpFilter`] trait.
impl<EHF: EnvoyHttpFilter> HttpFilter<EHF> for Filter {
    fn on_request_headers(
        &mut self,
        envoy_filter: &mut EHF,
        _end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_request_headers_status {
        envoy_log_trace!("on_request_headers");
        // TODO: need to test if we get called even if there is no transformation setting
        //       if yes, we need to short circuit here and return Continue
        if !_end_of_stream {
            // TODO: this here always stop iteration to wait for the full request body,
            //       need to support body passthrough
            return abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::StopIteration;
        }

        self.set_per_route_config(envoy_filter);
        // TODO(nfuden): find someone who knows rust to see if we really need this Hash map for serialization
        self.populate_request_headers_map(envoy_filter.get_request_headers());
        self.transform_request_headers(envoy_filter);
        abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue
    }

    fn on_request_body(
        &mut self,
        envoy_filter: &mut EHF,
        end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_request_body_status {
        envoy_log_trace!("on_request_body");
        // TODO: need to test if we get called even if there is no transformation setting
        //       if yes, we need to short circuit here and return Continue
        if !end_of_stream {
            // TODO: Technically, we don't need to buffer the body yet as we don't support parsing the body now
            //       but it will be coming next. This is mimicking the C++ transformation filter behavior to
            //       always buffer the request body by default unless passthrough is set. Will revisit and consider
            //       if this is the desired behavior when we implement parsing the body
            return abi::envoy_dynamic_module_type_on_http_filter_request_body_status::StopIterationAndBuffer;
        }

        self.set_per_route_config(envoy_filter);
        // TODO(nfuden): find someone who knows rust to see if we really need this Hash map for serialization
        self.populate_request_headers_map(envoy_filter.get_request_headers());
        self.transform_request_headers(envoy_filter);
        abi::envoy_dynamic_module_type_on_http_filter_request_body_status::Continue
    }

    fn on_response_headers(
        &mut self,
        envoy_filter: &mut EHF,
        _end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_response_headers_status {
        envoy_log_trace!("on_response_headers");
        self.set_per_route_config(envoy_filter);
        // TODO(nfuden): find someone who knows rust to see if we really need this Hash map for serialization
        self.populate_request_headers_map(envoy_filter.get_request_headers());
        self.transform_response_headers(envoy_filter);
        abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::Continue
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn test_injected_functions() {
        // get envoy's mockall impl for httpfilter
        let mut envoy_filter = envoy_proxy_dynamic_modules_rust_sdk::MockEnvoyHttpFilter::default();

        // construct the filter config
        // most upstream tests start with the filter itself but we are tryign to add heavier logic
        // to the config factory strat rather than running it on header calls
        let mut filter_conf = FilterConfig {
            request_headers_setter: vec![
                (
                    "X-substring".to_string(),
                    "{{substring(\"ENVOYPROXY something\", 5, 10) }}".to_string(),
                ),
                (
                    "X-substring-no-3rd".to_string(),
                    "{{substring(\"ENVOYPROXY something\", 5) }}".to_string(),
                ),
                (
                    "X-donor-header-contents".to_string(),
                    "{{ header(\"x-donor\") }}".to_string(),
                ),
                (
                    "X-donor-header-substringed".to_string(),
                    "{{ substring( header(\"x-donor\"), 0, 7)}}".to_string(),
                ),
            ],
            response_headers_setter: vec![("X-Bar".to_string(), "foo".to_string())],
        };
        let mut filter = filter_conf.new_http_filter(&mut envoy_filter);

        envoy_filter
            .expect_get_most_specific_route_config()
            .returning(|| None);

        envoy_filter.expect_get_request_headers().returning(|| {
            vec![
                (EnvoyBuffer::new("host"), EnvoyBuffer::new("example.com")),
                (
                    EnvoyBuffer::new("x-donor"),
                    EnvoyBuffer::new("thedonorvalue"),
                ),
            ]
        });

        envoy_filter.expect_get_response_headers().returning(|| {
            vec![
                (EnvoyBuffer::new("host"), EnvoyBuffer::new("example.com")),
                (
                    EnvoyBuffer::new("x-donor"),
                    EnvoyBuffer::new("thedonorvalue"),
                ),
            ]
        });

        let mut seq = Sequence::new();
        envoy_filter
            .expect_set_request_header()
            .times(1)
            .in_sequence(&mut seq)
            .returning(|key, value: &[u8]| {
                assert_eq!(key, "X-substring");
                assert_eq!(std::str::from_utf8(value).unwrap(), "PROXY");
                true
            });

        envoy_filter
            .expect_set_request_header()
            .times(1)
            .in_sequence(&mut seq)
            .returning(|key, value: &[u8]| {
                assert_eq!(key, "X-substring-no-3rd");
                assert_eq!(std::str::from_utf8(value).unwrap(), "PROXY something");
                true
            });

        envoy_filter
            .expect_set_request_header()
            .times(1)
            .in_sequence(&mut seq)
            .returning(|key, value: &[u8]| {
                assert_eq!(key, "X-donor-header-contents");
                assert_eq!(std::str::from_utf8(value).unwrap(), "thedonorvalue");
                true
            });

        envoy_filter
            .expect_set_request_header()
            .times(1)
            .in_sequence(&mut seq)
            .returning(|key, value: &[u8]| {
                assert_eq!(key, "X-donor-header-substringed");
                assert_eq!(std::str::from_utf8(value).unwrap(), "thedono");
                true
            });

        envoy_filter
            .expect_set_response_header()
            .returning(|key, value| {
                assert_eq!(key, "X-Bar");
                assert_eq!(value, b"foo");
                true
            });

        assert_eq!(
            filter.on_request_headers(&mut envoy_filter, true),
            abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue
        );
        assert_eq!(
            filter.on_response_headers(&mut envoy_filter, true),
            abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::Continue
        );
    }
    #[test]
    fn test_minininja_functionality() {
        // get envoy's mockall impl for httpfilter
        let mut envoy_filter = envoy_proxy_dynamic_modules_rust_sdk::MockEnvoyHttpFilter::default();

        // construct the filter config
        // most upstream tests start with the filter itself but we are trying to add heavier logic
        // to the config factory start rather than running it on header calls
        let mut filter_conf = FilterConfig {
            request_headers_setter: vec![(
                "X-if-truth".to_string(),
                "{%- if true -%}supersuper{% endif %}".to_string(),
            )],
            response_headers_setter: vec![("X-Bar".to_string(), "foo".to_string())],
        };
        let mut filter = filter_conf.new_http_filter(&mut envoy_filter);

        envoy_filter
            .expect_get_most_specific_route_config()
            .returning(|| None);

        envoy_filter.expect_get_request_headers().returning(|| {
            vec![
                (EnvoyBuffer::new("host"), EnvoyBuffer::new("example.com")),
                (
                    EnvoyBuffer::new("x-donor"),
                    EnvoyBuffer::new("thedonorvalue"),
                ),
            ]
        });

        envoy_filter.expect_get_response_headers().returning(|| {
            vec![
                (EnvoyBuffer::new("host"), EnvoyBuffer::new("example.com")),
                (
                    EnvoyBuffer::new("x-donor"),
                    EnvoyBuffer::new("thedonorvalue"),
                ),
            ]
        });

        let mut seq = Sequence::new();
        envoy_filter
            .expect_set_request_header()
            .times(1)
            .in_sequence(&mut seq)
            .returning(|key, value: &[u8]| {
                assert_eq!(key, "X-if-truth");
                assert_eq!(std::str::from_utf8(value).unwrap(), "supersuper");
                true
            });
        envoy_filter
            .expect_set_response_header()
            .returning(|key, value| {
                assert_eq!(key, "X-Bar");
                assert_eq!(value, b"foo");
                true
            });
        assert_eq!(
            filter.on_request_headers(&mut envoy_filter, false),
            abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::StopIteration
        );
        assert_eq!(
            filter.on_request_headers(&mut envoy_filter, true),
            abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue
        );
        assert_eq!(
            filter.on_response_headers(&mut envoy_filter, true),
            abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::Continue
        );
    }
}
