use anyhow::{Context, Result};
use envoy_proxy_dynamic_modules_rust_sdk::*;
use minijinja::Environment;
use once_cell::sync::Lazy;
use serde_json::Value as JsonValue;
use std::collections::HashMap;
use transformations::{
    LocalTransform, LocalTransformationConfig, TransformationError, TransformationOps,
};

#[cfg(test)]
use mockall::*;

static EMPTY_MAP: Lazy<HashMap<String, String>> = Lazy::new(HashMap::new);
#[derive(Clone)]
pub struct FilterConfig {
    transformations: LocalTransformationConfig,
    env: Environment<'static>,
}

struct EnvoyTransformationOps<'a> {
    envoy_filter: &'a mut dyn EnvoyHttpFilter,
    used_received_response_body: Option<bool>,
}

impl<'a> EnvoyTransformationOps<'a> {
    fn new(envoy_filter: &'a mut dyn EnvoyHttpFilter) -> EnvoyTransformationOps<'a> {
        EnvoyTransformationOps {
            envoy_filter,
            used_received_response_body: None,
        }
    }
}
impl TransformationOps for EnvoyTransformationOps<'_> {
    // REMOVE-ENVOY-1.37 : after upgrading to envoy 1.37, remove the platform specific directive here
    //                     and the no-op add_request_header()
    #[cfg(target_arch = "x86_64")]
    fn add_request_header(&mut self, key: &str, value: &[u8]) -> bool {
        self.envoy_filter.add_request_header(key, value)
    }

    #[cfg(not(target_arch = "x86_64"))]
    fn add_request_header(&mut self, _key: &str, _value: &[u8]) -> bool {
        envoy_log_warn!("add header is currently not supported for non-x86 build. set header can be used if existing header can be overwritten.");
        true
    }

    fn set_request_header(&mut self, key: &str, value: &[u8]) -> bool {
        self.envoy_filter.set_request_header(key, value)
    }
    fn remove_request_header(&mut self, key: &str) -> bool {
        self.envoy_filter.remove_request_header(key)
    }
    fn parse_request_json_body(&mut self) -> Result<JsonValue> {
        let Some(buffers) = self.envoy_filter.get_buffered_request_body() else {
            return Ok(JsonValue::Null);
        };

        if buffers.is_empty() {
            return Ok(JsonValue::Null);
        }
        // TODO: implement Reader for EnvoyBuffer and use serde_json::from_reader to avoid making copy first?
        let chunks: Vec<_> = buffers.iter().map(|b| b.as_slice()).collect();
        let body = chunks.concat();
        serde_json::from_slice(&body).context("failed to parse request body as json")
    }
    fn get_request_body(&mut self) -> Vec<u8> {
        let Some(buffers) = self.envoy_filter.get_buffered_request_body() else {
            return Vec::default();
        };

        // TODO: implement Reader for EnvoyBuffer and use serde_json::from_reader to avoid making copy first?
        let chunks: Vec<_> = buffers.iter().map(|b| b.as_slice()).collect();
        chunks.concat()
    }
    fn drain_request_body(&mut self, number_of_bytes: usize) -> bool {
        self.envoy_filter
            .drain_buffered_request_body(number_of_bytes)
    }
    fn append_request_body(&mut self, data: &[u8]) -> bool {
        self.envoy_filter.append_buffered_request_body(data)
    }

    // REMOVE-ENVOY-1.37 : after upgrading to envoy 1.37, remove the platform specific directive here
    //                     and the no-op add_response_header()
    #[cfg(target_arch = "x86_64")]
    fn add_response_header(&mut self, key: &str, value: &[u8]) -> bool {
        self.envoy_filter.add_response_header(key, value)
    }
    #[cfg(not(target_arch = "x86_64"))]
    fn add_response_header(&mut self, _key: &str, _value: &[u8]) -> bool {
        envoy_log_warn!("add header is currently not supported for non-x86 build. set header can be used if existing header can be overwritten.");
        true
    }
    fn set_response_header(&mut self, key: &str, value: &[u8]) -> bool {
        self.envoy_filter.set_response_header(key, value)
    }
    fn remove_response_header(&mut self, key: &str) -> bool {
        self.envoy_filter.remove_response_header(key)
    }
    fn parse_response_json_body(&mut self) -> Result<JsonValue> {
        let body = self.get_response_body();
        if body.is_empty() {
            return Ok(JsonValue::Null);
        }
        serde_json::from_slice(&body).context("failed to parse response body as json")
    }
    fn get_response_body(&mut self) -> Vec<u8> {
        self.used_received_response_body = Some(false);

        let mut buffers = self.envoy_filter.get_buffered_response_body();

        if buffers.is_none() {
            // For LocalReply, the body is in the "received_response_body"
            buffers = self.envoy_filter.get_received_response_body();
            if buffers.is_some() {
                self.used_received_response_body = Some(true);
            }
        }

        match buffers {
            None => Vec::default(),
            Some(buffers) => {
                // TODO: implement Reader for EnvoyBuffer and use serde_json::from_reader to avoid making copy first?
                let chunks: Vec<_> = buffers.iter().map(|b| b.as_slice()).collect();
                chunks.concat()
            }
        }
    }
    fn drain_response_body(&mut self, number_of_bytes: usize) -> bool {
        // With testing, it seems to be unnecessary to detect
        // if we should drain the "received_response_body" if the body is from there.
        // As long as something get pushed to the "buffered_response_body", that
        // seems to get used first before the received_response_body.

        // ie in the case of LocalReply, the body comes from "received_response_body"
        // but we can push the new body in "buffered_response_body" without draining
        // the "received_response_body", the new body is sent to end user.

        // However, not sure if there is any side effect for that, so doing it here
        // just in case.
        if self.used_received_response_body.is_none() {
            // the used_received_response_body boolean only get set if
            // the body() inja function is used in the transformation
            // so, detect it here again if not set.
            self.used_received_response_body = Some(false);
            if self.envoy_filter.get_buffered_response_body().is_none()
                && self.envoy_filter.get_received_response_body().is_some()
            {
                self.used_received_response_body = Some(true);
            }
        }

        if self.used_received_response_body.unwrap_or(false) {
            self.envoy_filter
                .drain_received_response_body(number_of_bytes)
        } else {
            self.envoy_filter
                .drain_buffered_response_body(number_of_bytes)
        }
    }
    fn append_response_body(&mut self, data: &[u8]) -> bool {
        if self.used_received_response_body.unwrap_or(false) {
            self.envoy_filter.append_received_response_body(data)
        } else {
            self.envoy_filter.append_buffered_response_body(data)
        }
    }
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
                envoy_log_error!("error parsing filter config: {filter_config} {err}");
                return None;
            }
        };

        let env = match transformations::jinja::create_env_with_templates(&config) {
            Ok(env) => env,
            Err(err) => {
                envoy_log_error!("error compiling templates: {err}");
                return None;
            }
        };

        Some(FilterConfig {
            transformations: config,
            env,
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
            request_headers_map: None,
        })
    }
}

pub struct Filter {
    filter_config: FilterConfig,
    per_route_config: Option<Box<PerRouteConfig>>,
    request_headers_map: Option<HashMap<String, String>>,
}

impl Filter {
    fn get_env(&self) -> &Environment<'static> {
        match self.get_per_route_config() {
            Some(config) => &config.env,
            None => &self.filter_config.env,
        }
    }

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

    // set_per_route_config() has to be called before calling this function
    fn get_request_transform(&self) -> &Option<LocalTransform> {
        match self.get_per_route_config() {
            Some(config) => &config.transformations.request,
            None => &self.filter_config.transformations.request,
        }
    }

    // set_per_route_config() has to be called before calling this function
    fn has_request_transform(&self) -> bool {
        let Some(transform) = self.get_request_transform() else {
            return false;
        };

        !transform.is_empty()
    }

    // set_per_route_config() has to be called before calling this function
    fn get_response_transform(&self) -> &Option<LocalTransform> {
        match self.get_per_route_config() {
            Some(config) => &config.transformations.response,
            None => &self.filter_config.transformations.response,
        }
    }

    // set_per_route_config() has to be called before calling this function
    fn has_response_transform(&self) -> bool {
        let Some(transform) = self.get_response_transform() else {
            return false;
        };

        !transform.is_empty()
    }

    fn transform_request<EHF: EnvoyHttpFilter>(&self, envoy_filter: &mut EHF) -> bool {
        if let Some(transform) = self.get_request_transform() {
            match transformations::jinja::transform_request(
                self.get_env(),
                transform,
                self.get_request_headers_map(),
                EnvoyTransformationOps::new(envoy_filter),
            ) {
                Ok(()) => {}
                Err(err) => {
                    if let Some(e) = err.downcast_ref::<TransformationError>() {
                        match e {
                            TransformationError::UndeclaredJsonVariables(_msg) => {
                                envoy_log_error!("{:#}", err);
                                envoy_filter.send_response(400, Vec::default(), None);
                                return false;
                            }
                        }
                    } else if let Some(e) = err.downcast_ref::<serde_json::error::Error>() {
                        envoy_log_error!("json parsing error: {:#}", e);
                        envoy_filter.send_response(400, Vec::default(), None);
                        return false;
                    } else {
                        envoy_log_warn!("{:#}", err);
                    }
                }
            }
        }

        true
    }

    fn transform_response<EHF: EnvoyHttpFilter>(&self, envoy_filter: &mut EHF) -> bool {
        if let Some(transform) = self.get_response_transform() {
            let response_headers_map = self.create_headers_map(envoy_filter.get_response_headers());

            match transformations::jinja::transform_response(
                self.get_env(),
                transform,
                self.get_request_headers_map(),
                &response_headers_map,
                EnvoyTransformationOps::new(envoy_filter),
            ) {
                Ok(()) => {}
                Err(err) => {
                    if let Some(e) = err.downcast_ref::<TransformationError>() {
                        match e {
                            TransformationError::UndeclaredJsonVariables(_msg) => {
                                envoy_log_error!("{:#}", err);
                                envoy_filter.send_response(400, Vec::default(), None);
                                return false;
                            }
                        }
                    } else if let Some(e) = err.downcast_ref::<serde_json::error::Error>() {
                        envoy_log_error!("json parsing error: {:#}", e);
                        envoy_filter.send_response(400, Vec::default(), None);
                        return false;
                    } else {
                        envoy_log_warn!("{:#}", err);
                    }
                }
            }
        }

        true
    }
}

/// This implements the [`envoy_proxy_dynamic_modules_rust_sdk::HttpFilter`] trait.
impl<EHF: EnvoyHttpFilter> HttpFilter<EHF> for Filter {
    fn on_request_headers(
        &mut self,
        envoy_filter: &mut EHF,
        _end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_request_headers_status {
        self.set_per_route_config(envoy_filter);
        if !self.has_request_transform() {
            envoy_log_trace!("on_request_headers skipping");
            return abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue;
        }

        if !_end_of_stream {
            // TODO: this here always stop iteration to wait for the full request body,
            //       need to support body passthrough
            envoy_log_trace!("on_request_headers buffering");
            //            return abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::StopAllIterationAndBuffer;
            return abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::StopIteration;
        }
        envoy_log_trace!("on_request_headers");

        self.populate_request_headers_map(envoy_filter.get_request_headers());
        if self.transform_request(envoy_filter) {
            return abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue;
        }

        // If transform had a critical error, it would have sent a local reply with 400 already,
        // so return StopIteration here
        abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::StopIteration
    }

    fn on_request_body(
        &mut self,
        envoy_filter: &mut EHF,
        end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_request_body_status {
        self.set_per_route_config(envoy_filter);
        if !self.has_request_transform() {
            envoy_log_trace!("on_request_body skipping");
            return abi::envoy_dynamic_module_type_on_http_filter_request_body_status::Continue;
        }

        if !end_of_stream {
            envoy_log_trace!("on_request_body buffering");
            // This is mimicking the C++ transformation filter behavior to always buffer the request body by
            // default unless passthrough is set but kgateway doesn't support body passthrough in
            // transformation API.
            return abi::envoy_dynamic_module_type_on_http_filter_request_body_status::StopIterationAndBuffer;
        }
        envoy_log_trace!("on_request_body");

        self.populate_request_headers_map(envoy_filter.get_request_headers());
        if self.transform_request(envoy_filter) {
            return abi::envoy_dynamic_module_type_on_http_filter_request_body_status::Continue;
        }

        // If transform had a critical error, it would have sent a local reply with 400 already,
        // so return StopIteration here
        abi::envoy_dynamic_module_type_on_http_filter_request_body_status::StopIterationAndBuffer
    }

    fn on_response_headers(
        &mut self,
        envoy_filter: &mut EHF,
        end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_response_headers_status {
        self.set_per_route_config(envoy_filter);
        if !self.has_response_transform() {
            envoy_log_trace!("on_response_header skipping");
            return abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::Continue;
        }

        if !end_of_stream {
            envoy_log_trace!("on_response_headers buffering");
            return abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::StopIteration;
        }
        envoy_log_trace!("on_response_headers");
        self.populate_request_headers_map(envoy_filter.get_request_headers());
        if self.transform_response(envoy_filter) {
            return abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::Continue;
        }

        // If transform had a critical error, it would have sent a local reply with 400 already,
        // so return StopIteration here
        abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::StopIteration
    }

    fn on_response_body(
        &mut self,
        envoy_filter: &mut EHF,
        end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_response_body_status {
        self.set_per_route_config(envoy_filter);
        if !self.has_response_transform() {
            envoy_log_trace!("on_response_body skipping");
            return abi::envoy_dynamic_module_type_on_http_filter_response_body_status::Continue;
        }
        if !end_of_stream {
            envoy_log_trace!("on_response_body buffering");
            // This is mimicking the C++ transformation filter behavior to always buffer the response body by
            // default unless passthrough is set but kgateway doesn't support body passthrough in
            // transformation API.
            return abi::envoy_dynamic_module_type_on_http_filter_response_body_status::StopIterationAndBuffer;
        }
        envoy_log_trace!("on_response_body");

        self.populate_request_headers_map(envoy_filter.get_request_headers());
        if self.transform_response(envoy_filter) {
            return abi::envoy_dynamic_module_type_on_http_filter_response_body_status::Continue;
        }

        // If transform had a critical error, it would have sent a local reply with 400 already,
        // so return StopIteration here
        abi::envoy_dynamic_module_type_on_http_filter_response_body_status::StopIterationAndBuffer
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
              { "name": "X-substring", "value": "{{substring(\"ENVOYPROXY something\", 5, 5) }}" },
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
