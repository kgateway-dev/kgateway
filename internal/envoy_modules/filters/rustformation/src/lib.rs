#![deny(clippy::unwrap_used, clippy::expect_used)]

use anyhow::{Context, Result};
use envoy_helpers::{
    http::{create_headers_map, detect_upgrade_request},
    EnvoyBuffersReader,
};
use envoy_proxy_dynamic_modules_rust_sdk::*;
use minijinja::Environment;
use once_cell::sync::Lazy;
use serde_json::Value as JsonValue;
use std::collections::HashMap;
use transformation::{
    jinja::ProcessFlags, LocalTransform, LocalTransformationConfig, TransformationError,
    TransformationOps,
};

static EMPTY_MAP: Lazy<HashMap<String, String>> = Lazy::new(HashMap::new);
#[derive(Clone)]
pub struct FilterConfig {
    transformations: LocalTransformationConfig,
    env: Environment<'static>,
}

struct EnvoyTransformationOps<'a, EHF: EnvoyHttpFilter> {
    envoy_filter: &'a mut EHF,
    used_received_request_body: Option<bool>,
    used_received_response_body: Option<bool>,
}

impl<'a, EHF: EnvoyHttpFilter> EnvoyTransformationOps<'a, EHF> {
    fn new(envoy_filter: &'a mut EHF) -> EnvoyTransformationOps<'a, EHF> {
        EnvoyTransformationOps {
            envoy_filter,
            used_received_request_body: None,
            used_received_response_body: None,
        }
    }
}
impl<EHF: EnvoyHttpFilter> TransformationOps for EnvoyTransformationOps<'_, EHF> {
    fn add_request_header(&mut self, key: &str, value: &[u8]) -> bool {
        self.envoy_filter.add_request_header(key, value)
    }

    fn set_request_header(&mut self, key: &str, value: &[u8]) -> bool {
        self.envoy_filter.set_request_header(key, value)
    }
    fn remove_request_header(&mut self, key: &str) -> bool {
        self.envoy_filter.remove_request_header(key)
    }
    fn parse_request_json_body(&mut self) -> Result<JsonValue> {
        use std::io::Read as _;
        let mut reader = self.get_request_body_reader();
        let mut peek = [0u8; 1];
        if reader.read(&mut peek)? == 0 {
            return Ok(JsonValue::Null);
        }
        let chained = std::io::Cursor::new(peek).chain(reader);
        serde_json::from_reader(chained).context("failed to parse request body as json")
    }
    fn get_request_body_reader(&mut self) -> Box<dyn std::io::Read + '_> {
        self.used_received_request_body = Some(false);

        // Check buffered first; if None, fall back to received.
        // TODO: in envoy v1.38, there is a function received_buffered_request_body()
        //       to check if it's buffered
        if self.envoy_filter.get_buffered_request_body().is_some() {
            // Called twice to avoid holding the borrow across the is_some() check.
            // The unwrap is safe: is_some() was just verified above.
            #[allow(clippy::unwrap_used)]
            let buffers = self.envoy_filter.get_buffered_request_body().unwrap();
            return Box::new(EnvoyBuffersReader::new(buffers));
        }

        // When the body arrives in a single chunk (common for small JSON
        // payloads), the first on_request_body callback fires with
        // end_of_stream=true before any prior StopIterationAndBuffer could
        // populate the buffered body.  In that case the data is only in the
        // "received" buffer — mirror the same fallback used for responses.
        match self.envoy_filter.get_received_request_body() {
            Some(buffers) => {
                self.used_received_request_body = Some(true);
                Box::new(EnvoyBuffersReader::new(buffers))
            }
            None => Box::new(std::io::empty()),
        }
    }
    fn get_request_body(&mut self) -> Vec<u8> {
        // TODO: switch to use read_whole_request_body() after upgrading to v1.38 envoy
        let mut body = Vec::new();
        self.get_request_body_reader()
            .read_to_end(&mut body)
            .unwrap_or_else(|e| {
                envoy_log_warn!("failed to read request body: {e}");
                0
            });
        body
    }
    fn drain_request_body(&mut self, number_of_bytes: usize) -> bool {
        if self.used_received_request_body.is_none() {
            self.used_received_request_body = Some(false);
            if self.envoy_filter.get_buffered_request_body().is_none()
                && self.envoy_filter.get_received_request_body().is_some()
            {
                self.used_received_request_body = Some(true);
            }
        }

        if self.used_received_request_body.unwrap_or(false) {
            self.envoy_filter
                .drain_received_request_body(number_of_bytes)
        } else {
            self.envoy_filter
                .drain_buffered_request_body(number_of_bytes)
        }
    }
    fn append_request_body(&mut self, data: &[u8]) -> bool {
        if self.used_received_request_body.unwrap_or(false) {
            self.envoy_filter.append_received_request_body(data)
        } else {
            self.envoy_filter.append_buffered_request_body(data)
        }
    }

    fn add_response_header(&mut self, key: &str, value: &[u8]) -> bool {
        self.envoy_filter.add_response_header(key, value)
    }
    fn set_response_header(&mut self, key: &str, value: &[u8]) -> bool {
        self.envoy_filter.set_response_header(key, value)
    }
    fn remove_response_header(&mut self, key: &str) -> bool {
        self.envoy_filter.remove_response_header(key)
    }
    fn parse_response_json_body(&mut self) -> Result<JsonValue> {
        use std::io::Read as _;
        let mut reader = self.get_response_body_reader();
        let mut peek = [0u8; 1];
        if reader.read(&mut peek)? == 0 {
            return Ok(JsonValue::Null);
        }
        let chained = std::io::Cursor::new(peek).chain(reader);
        serde_json::from_reader(chained).context("failed to parse response body as json")
    }
    fn get_response_body_reader(&mut self) -> Box<dyn std::io::Read + '_> {
        self.used_received_response_body = Some(false);

        // Check buffered first; if None, fall back to received.
        // TODO: in envoy v1.38, there is a function received_buffered_response_body()
        //       to check if it's buffered
        if self.envoy_filter.get_buffered_response_body().is_some() {
            // Called twice to avoid holding the borrow across the is_some() check.
            // The unwrap is safe: is_some() was just verified above.
            #[allow(clippy::unwrap_used)]
            let buffers = self.envoy_filter.get_buffered_response_body().unwrap();
            return Box::new(EnvoyBuffersReader::new(buffers));
        }

        // For LocalReply, the body is in the "received_response_body"
        match self.envoy_filter.get_received_response_body() {
            Some(buffers) => {
                self.used_received_response_body = Some(true);
                Box::new(EnvoyBuffersReader::new(buffers))
            }
            None => Box::new(std::io::empty()),
        }
    }
    fn get_response_body(&mut self) -> Vec<u8> {
        // TODO: switch to use read_whole_response_body() after upgrading to v1.38 envoy
        let mut body = Vec::new();
        self.get_response_body_reader()
            .read_to_end(&mut body)
            .unwrap_or_else(|e| {
                envoy_log_warn!("failed to read response body: {e}");
                0
            });
        body
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
    fn set_dynamic_metadata_string(&mut self, namespace: &str, key: &str, value: &str) {
        self.envoy_filter
            .set_dynamic_metadata_string(namespace, key, value);
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

        let env = match transformation::jinja::create_env_with_templates(&config) {
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
    fn new_http_filter(&self, _envoy: &mut EHF) -> Box<dyn HttpFilter<EHF>> {
        Box::new(Filter {
            filter_config: self.clone(),
            per_route_config: None,
            request_headers_map: None,
            is_upgrade_request: false,
        })
    }
}

pub struct Filter {
    filter_config: FilterConfig,
    per_route_config: Option<Box<PerRouteConfig>>,
    request_headers_map: Option<HashMap<String, String>>,
    is_upgrade_request: bool,
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

    // This function is used to populate the self.request_headers_map so we only ever
    // do it once while we might need the request headers in either on_request_headers() or
    // on_response_headers().
    fn populate_request_headers_map(&mut self, headers: Vec<(EnvoyBuffer, EnvoyBuffer)>) {
        if self.request_headers_map.is_none() {
            self.request_headers_map = Some(create_headers_map(headers));
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
    fn get_response_transform(&self) -> &Option<LocalTransform> {
        match self.get_per_route_config() {
            Some(config) => &config.transformations.response,
            None => &self.filter_config.transformations.response,
        }
    }

    // Returns true if buffering should be skipped, either because the transform
    // itself requests it or because the request is an upgrade/tunnel request.
    fn skip_buffering(&self, transform: &LocalTransform) -> bool {
        self.is_upgrade_request || transform.skip_buffering()
    }

    // Handles a transformation error by sending a 400 response for known error
    // types. Returns false if the caller should abort (response was sent), true
    // if processing can continue.
    fn handle_transform_error<EHF: EnvoyHttpFilter>(
        err: anyhow::Error,
        envoy_filter: &mut EHF,
    ) -> bool {
        if let Some(e) = err.downcast_ref::<TransformationError>() {
            match e {
                TransformationError::UndeclaredJsonVariables(_msg) => {
                    envoy_log_error!("{:#}", err);
                    envoy_filter.send_response(
                        400,
                        Vec::default(),
                        None,
                        Some("undeclared json variables in transformation template"),
                    );
                    return false;
                }
            }
        } else if let Some(e) = err.downcast_ref::<serde_json::error::Error>() {
            envoy_log_error!("json parsing error: {:#}", e);
            envoy_filter.send_response(
                400,
                Vec::default(),
                None,
                Some("json parsing error in transformation template"),
            );
            return false;
        } else {
            envoy_log_warn!("{:#}", err);
        }
        true
    }

    fn transform_request<EHF: EnvoyHttpFilter>(
        &self,
        envoy_filter: &mut EHF,
        flags: ProcessFlags,
    ) -> bool {
        let Some(transform) = self.get_request_transform() else {
            return true;
        };

        let mut retval = true;
        match transformation::jinja::transform_request(
            self.get_env(),
            transform,
            self.get_request_headers_map(),
            flags,
            EnvoyTransformationOps::new(envoy_filter),
        ) {
            Ok(()) => {}
            Err(err) => {
                retval = Filter::handle_transform_error(err, envoy_filter);
            }
        }

        retval
    }

    fn transform_response<EHF: EnvoyHttpFilter>(
        &self,
        envoy_filter: &mut EHF,
        flags: ProcessFlags,
    ) -> bool {
        let Some(transform) = self.get_response_transform() else {
            return true;
        };

        let response_headers_map = create_headers_map(envoy_filter.get_response_headers());

        let mut retval = true;
        match transformation::jinja::transform_response(
            self.get_env(),
            transform,
            self.get_request_headers_map(),
            &response_headers_map,
            flags,
            EnvoyTransformationOps::new(envoy_filter),
        ) {
            Ok(()) => {}
            Err(err) => {
                retval = Filter::handle_transform_error(err, envoy_filter);
            }
        }

        retval
    }
}

/// This implements the [`envoy_proxy_dynamic_modules_rust_sdk::HttpFilter`] trait.
impl<EHF: EnvoyHttpFilter> HttpFilter<EHF> for Filter {
    fn on_request_headers(
        &mut self,
        envoy_filter: &mut EHF,
        end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_request_headers_status {
        self.set_per_route_config(envoy_filter);
        self.populate_request_headers_map(envoy_filter.get_request_headers());
        self.is_upgrade_request = detect_upgrade_request(self.get_request_headers_map());

        let Some(transform) = self.get_request_transform() else {
            envoy_log_trace!("on_request_headers skipping");
            return abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue;
        };
        if transform.is_empty() {
            envoy_log_trace!("on_request_headers skipping");
            return abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue;
        }

        let skip_buffering = self.skip_buffering(transform);
        if skip_buffering {
            // when we skip_buffering, we ignore any body transformation,
            // so only do the header transformation and continue. Technically,
            // we could inject body from transformation if end_of_stream is true
            // but that might create confusion on when body transformation will be
            // ignored. So, for now, just ignore all body transformation if parseAs: None
            // is set
            envoy_log_trace!("on_request_headers transform_request skip buffering");
            if self.transform_request(envoy_filter, ProcessFlags::HEADER) {
                return abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue;
            }
        } else if end_of_stream {
            // when we are NOT skip_buffering but it's end of stream, there is no body from the
            // request but the transformation might inject one, so we need to process both HEADER
            // and BODY for transformation
            envoy_log_trace!("on_request_headers transform_request eos");
            if self.transform_request(envoy_filter, ProcessFlags::HEADER | ProcessFlags::BODY) {
                return abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue;
            }
        } else {
            // This is where we are not skip_buffering but there is a body, stop the iteration
            // so we can buffer all data in on_request_body before continuing the filter chain
            envoy_log_trace!("on_request_headers buffering");
            return abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::StopIteration;
        }

        // If transform had a critical error, it would have sent a local reply with 400 already,
        // so return StopIteration here
        envoy_log_trace!("on_request_headers transform_request failed");
        abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::StopIteration
    }

    fn on_request_body(
        &mut self,
        envoy_filter: &mut EHF,
        end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_request_body_status {
        self.set_per_route_config(envoy_filter);
        self.populate_request_headers_map(envoy_filter.get_request_headers());
        let Some(transform) = self.get_request_transform() else {
            envoy_log_trace!("on_request_body skipping");
            return abi::envoy_dynamic_module_type_on_http_filter_request_body_status::Continue;
        };
        if transform.is_empty() {
            envoy_log_trace!("on_request_body skipping");
            return abi::envoy_dynamic_module_type_on_http_filter_request_body_status::Continue;
        }
        if self.skip_buffering(transform) {
            envoy_log_trace!("on_request_body skipped buffering and processing");
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

        if self.transform_request(envoy_filter, ProcessFlags::HEADER | ProcessFlags::BODY) {
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
        self.populate_request_headers_map(envoy_filter.get_request_headers());
        let Some(transform) = self.get_response_transform() else {
            envoy_log_trace!("on_response_headers skipping");
            return abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::Continue;
        };
        if transform.is_empty() {
            envoy_log_trace!("on_response_headers skipping");
            return abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::Continue;
        }

        let skip_buffering = self.skip_buffering(transform);
        if skip_buffering {
            // when we skip_buffering, we ignore any body transformation,
            // so only do the header transformation and continue. Technically,
            // we could inject body from transformation if end_of_stream is true
            // but that might create confusion on when body transformation will be
            // ignored. So, for now, just ignore all body transformation if parseAs: None
            // is set
            envoy_log_trace!("on_response_headers transform_response skip buffering");
            if self.transform_response(envoy_filter, ProcessFlags::HEADER) {
                return abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::Continue;
            }
        } else if end_of_stream {
            // when we are NOT skip_buffering but it's end of stream, there is no body from the
            // response but the transformation might inject one, so we need to process both HEADER
            // and BODY for transformation
            envoy_log_trace!("on_response_headers transform_response eos");
            if self.transform_response(envoy_filter, ProcessFlags::HEADER | ProcessFlags::BODY) {
                return abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::Continue;
            }
        } else {
            // This is where we are not skip_buffering but there is a body, stop the iteration
            // so we can buffer all data in on_response_body before continuing the filter chain
            envoy_log_trace!("on_response_headers buffering");
            return abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::StopIteration;
        }

        // If transform had a critical error, it would have sent a local reply with 400 already,
        // so return StopIteration here
        envoy_log_trace!("on_response_headers transform_response failed");
        abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::StopIteration
    }

    fn on_response_body(
        &mut self,
        envoy_filter: &mut EHF,
        end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_response_body_status {
        self.set_per_route_config(envoy_filter);
        self.populate_request_headers_map(envoy_filter.get_request_headers());
        let Some(transform) = self.get_response_transform() else {
            envoy_log_trace!("on_response_body skipping");
            return abi::envoy_dynamic_module_type_on_http_filter_response_body_status::Continue;
        };
        if transform.is_empty() {
            envoy_log_trace!("on_response_body skipping");
            return abi::envoy_dynamic_module_type_on_http_filter_response_body_status::Continue;
        }
        if self.skip_buffering(transform) {
            envoy_log_trace!("on_response_body skipped buffering and processing");
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

        if self.transform_response(envoy_filter, ProcessFlags::HEADER | ProcessFlags::BODY) {
            return abi::envoy_dynamic_module_type_on_http_filter_response_body_status::Continue;
        }

        // If transform had a critical error, it would have sent a local reply with 400 already,
        // so return StopIteration here
        abi::envoy_dynamic_module_type_on_http_filter_response_body_status::StopIterationAndBuffer
    }
}

#[cfg(test)]
mod tests;
