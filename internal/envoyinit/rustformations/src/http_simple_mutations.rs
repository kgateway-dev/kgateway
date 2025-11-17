use envoy_proxy_dynamic_modules_rust_sdk::*;
use lazy_static::lazy_static;
use serde::Deserialize;
use std::collections::HashMap;
use transformations::{LocalTransformationConfig, TransformationOps};

#[cfg(test)]
use mockall::*;

lazy_static! {
    static ref EMPTY_MAP: HashMap<String, String> = HashMap::new();
}
#[derive(Deserialize, Clone)]
pub struct FilterConfig {
    transformations: LocalTransformationConfig,
}

struct EnvoyTransformationOps<'a> {
    envoy_filter: &'a mut dyn EnvoyHttpFilter,
    //    TODO: see comment for get_random_pattern() below
    //    random_pattern_map: &'a mut Option<HashMap<String, String>>,
}

impl TransformationOps for EnvoyTransformationOps<'_> {
    fn set_request_header(&mut self, key: &str, value: &[u8]) -> bool {
        self.envoy_filter.set_request_header(key, value)
    }
    fn remove_request_header(&mut self, key: &str) -> bool {
        self.envoy_filter.remove_request_header(key)
    }
    fn set_response_header(&mut self, key: &str, value: &[u8]) -> bool {
        self.envoy_filter.set_response_header(key, value)
    }
    fn remove_response_header(&mut self, key: &str) -> bool {
        self.envoy_filter.remove_response_header(key)
    }
    /*
       TODO: was trying to use this to store the pattern in the request context that can be re-used
             for all replace_with_random() custom function but have not been able to find a way to
             do that yet with rust and minijinja

       fn get_random_pattern(&mut self, key: &str) -> String {
           let map = self.random_pattern_map.get_or_insert_with(HashMap::new);

           if let Some(pattern) = map.get(key) {
               return pattern.clone();
           }

           let new_pattern = rand::thread_rng()
               .sample_iter(&Alphanumeric)
               .take(8)
               .map(char::from)
               .collect()

           map.insert(key.to_string(), new_pattern.clone());

           new_pattern
       }
    */
}

impl FilterConfig {
    /// This is the constructor for the [`FilterConfig`].
    ///
    /// filter_config is the filter config from the Envoy config here:
    /// https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/dynamic_modules/v3/dynamic_modules.proto#envoy-v3-api-msg-extensions-dynamic-modules-v3-dynamicmoduleconfig
    pub fn new(filter_config: &str) -> Option<Self> {
        let config: LocalTransformationConfig = match serde_json::from_str(filter_config) {
            Ok(cfg) => cfg,
            Err(err) => {
                // Dont panic if there is incorrect configuration
                envoy_log_error!("Error parsing filter config: {filter_config} {err}");
                return None;
            }
        };
        Some(FilterConfig {
            transformations: config,
        })
    }
}

// Since PerRouteConfig is the same as the FilterConfig, for now just just a type alias
pub type PerRouteConfig = FilterConfig;

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
            let Some(value) = std::str::from_utf8(val.as_slice()).ok() else {
                continue;
            };

            headers_map.insert(key.to_string(), value.to_string());
        }

        headers_map
    }

    // This function is used to populate the self.request_headers_map so we only ever
    // do it once while we might need the request headers in either on_request_headers() or
    // on_response_headers().
    fn populate_request_headers_map(&mut self, headers: Vec<(EnvoyBuffer, EnvoyBuffer)>) {
        if self.request_headers_map.is_none() {
            self.request_headers_map = Some(self.create_headers_map(headers));
        }
    }

    fn get_request_headers_map(&self) -> &HashMap<String, String> {
        self.request_headers_map.as_ref().unwrap_or(&EMPTY_MAP)
    }

    fn transform_request_headers<EHF: EnvoyHttpFilter>(&self, envoy_filter: &mut EHF) {
        let request_transform = match self.get_per_route_config() {
            Some(config) => &config.transformations.request,
            None => &self.filter_config.transformations.request,
        };

        if let Some(transform) = request_transform {
            if let Err(e) = transformations::jinja::transform_request_headers(
                transform,
                &self.env,
                self.get_request_headers_map(),
                EnvoyTransformationOps { envoy_filter },
            ) {
                envoy_log_warn!("{e}");
            }
        }
    }

    fn transform_response_headers<EHF: EnvoyHttpFilter>(&self, envoy_filter: &mut EHF) {
        let response_transform = match self.get_per_route_config() {
            Some(config) => &config.transformations.response,
            None => &self.filter_config.transformations.response,
        };

        if let Some(transform) = response_transform {
            // TODO(nfuden): find someone who knows rust to see if we really need this Hash map for serialization
            let response_headers_map = self.create_headers_map(envoy_filter.get_response_headers());

            if let Err(e) = transformations::jinja::transform_response_headers(
                transform,
                &self.env,
                self.get_request_headers_map(),
                &response_headers_map,
                EnvoyTransformationOps { envoy_filter },
            ) {
                envoy_log_warn!("{e}");
            }
        }
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
        // most upstream tests start with the filter itself but we are trying to add heavier logic
        // to the config factory start rather than running it on header calls
        let json_str = r#"
        {
          "request": {
            "set": [
              { "name": "X-substring", "value": "{{substring(\"ENVOYPROXY something\", 5, 10) }}" },
              { "name": "X-substring-no-3rd", "value": "{{substring(\"ENVOYPROXY something\", 5) }}" },
              { "name": "X-donor-header-contents", "value": "{{ header(\"x-donor\") }}" },
              { "name": "X-donor-header-substringed", "value": "{{ substring( header(\"x-donor\"), 0, 7)}}" }
            ]
          },
          "response": {
            "set": [
              { "name": "X-Bar", "value": "foo" }
            ]
          },
          "foo": "This is a fake field to make sure the parser will ignore an new fields from the control plane for compatibility"
        }
        "#;
        let mut filter_conf =
            FilterConfig::new(json_str).expect("Failed to parse filter config json: {json_str}");
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
        let json_str = r#"
        {
          "request": {
            "set": [
              { "name": "X-if-truth", "value": "{%- if true -%}supersuper{% endif %}" }
            ]
          },
          "response": {
            "set": [
                { "name": "X-Bar", "value": "foo" }
            ]
          }
        }
        "#;
        let mut filter_conf =
            FilterConfig::new(json_str).expect("Failed to parse filter config json: {json_str}");
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
