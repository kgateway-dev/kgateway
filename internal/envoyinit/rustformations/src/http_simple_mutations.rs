use anyhow::{Context, Result};
use envoy_proxy_dynamic_modules_rust_sdk::*;
use minijinja::Environment;
use once_cell::sync::Lazy;
use serde_json::Value as JsonValue;
use std::collections::HashMap;
use transformations::{
    jinja::ProcessFlags, LocalTransform, LocalTransformationConfig, TransformationError,
    TransformationOps,
};

#[cfg(test)]
use mockall::*;

static EMPTY_MAP: Lazy<HashMap<String, String>> = Lazy::new(HashMap::new);
#[derive(Clone)]
pub struct FilterConfig {
    transformations: LocalTransformationConfig,
    env: Environment<'static>,
}

struct EnvoyBuffersReader<'a> {
    buffers: Vec<EnvoyMutBuffer<'a>>,
    chunk_idx: usize,
    offset: usize,
}

impl<'a> EnvoyBuffersReader<'a> {
    fn new(buffers: Vec<EnvoyMutBuffer<'a>>) -> Self {
        Self {
            buffers,
            chunk_idx: 0,
            offset: 0,
        }
    }
}

impl std::io::Read for EnvoyBuffersReader<'_> {
    fn read(&mut self, buf: &mut [u8]) -> std::io::Result<usize> {
        let mut filled = 0;
        while filled < buf.len() && self.chunk_idx < self.buffers.len() {
            let chunk = self.buffers[self.chunk_idx].as_slice();
            let remaining = &chunk[self.offset..];
            if remaining.is_empty() {
                self.chunk_idx += 1;
                self.offset = 0;
                continue;
            }
            let n = remaining.len().min(buf.len() - filled);
            buf[filled..filled + n].copy_from_slice(&remaining[..n]);
            self.offset += n;
            filled += n;
            if self.offset >= chunk.len() {
                self.chunk_idx += 1;
                self.offset = 0;
            }
        }
        Ok(filled)
    }
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
    fn get_response_transform(&self) -> &Option<LocalTransform> {
        match self.get_per_route_config() {
            Some(config) => &config.transformations.response,
            None => &self.filter_config.transformations.response,
        }
    }

    // Returns true if the request is a WebSocket upgrade or an HTTP CONNECT request,
    fn detect_upgrade_request(headers: &HashMap<String, String>) -> bool {
        if headers
            .get("upgrade")
            .is_some_and(|v| v.eq_ignore_ascii_case("websocket"))
        {
            return true;
        }
        if headers
            .get(":method")
            .is_some_and(|v| v.eq_ignore_ascii_case("connect"))
        {
            return true;
        }
        false
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
        match transformations::jinja::transform_request(
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

        let response_headers_map = self.create_headers_map(envoy_filter.get_response_headers());

        let mut retval = true;
        match transformations::jinja::transform_response(
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
        self.is_upgrade_request = Filter::detect_upgrade_request(self.get_request_headers_map());

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
mod tests {
    use super::*;
    use std::io::Read;

    // --- EnvoyBuffersReader unit tests ---

    #[test]
    fn test_envoy_buffers_reader_empty_buffers() {
        let mut reader = EnvoyBuffersReader::new(vec![]);
        let mut out = Vec::new();
        reader.read_to_end(&mut out).unwrap();
        assert!(out.is_empty());
    }

    #[test]
    #[allow(static_mut_refs)]
    fn test_envoy_buffers_reader_single_chunk() {
        static mut CHUNK: [u8; 5] = *b"hello";
        let buffers = vec![EnvoyMutBuffer::new(unsafe { &mut CHUNK })];
        let mut reader = EnvoyBuffersReader::new(buffers);
        let mut out = Vec::new();
        reader.read_to_end(&mut out).unwrap();
        assert_eq!(out, b"hello");
    }

    #[test]
    #[allow(static_mut_refs)]
    fn test_envoy_buffers_reader_multiple_chunks() {
        static mut CHUNK_X: [u8; 3] = *b"foo";
        static mut CHUNK_Y: [u8; 1] = *b"-";
        static mut CHUNK_Z: [u8; 3] = *b"bar";
        let buffers = vec![
            EnvoyMutBuffer::new(unsafe { &mut CHUNK_X }),
            EnvoyMutBuffer::new(unsafe { &mut CHUNK_Y }),
            EnvoyMutBuffer::new(unsafe { &mut CHUNK_Z }),
        ];
        let mut reader = EnvoyBuffersReader::new(buffers);
        let mut out = Vec::new();
        reader.read_to_end(&mut out).unwrap();
        assert_eq!(out, b"foo-bar");
    }

    #[test]
    #[allow(static_mut_refs)]
    fn test_envoy_buffers_reader_small_read_buf() {
        // Read buffer smaller than a single chunk — verifies partial-read and
        // offset advancement within a chunk.
        static mut CHUNK_X: [u8; 6] = *b"abcdef";
        static mut CHUNK_Y: [u8; 5] = *b"ghijk";
        let buffers = vec![
            EnvoyMutBuffer::new(unsafe { &mut CHUNK_X }),
            EnvoyMutBuffer::new(unsafe { &mut CHUNK_Y }),
        ];
        let mut reader = EnvoyBuffersReader::new(buffers);

        let mut tmp = [0u8; 4];
        let n1 = reader.read(&mut tmp).unwrap();
        assert_eq!(n1, 4);
        assert_eq!(&tmp[..n1], b"abcd");

        // read across chunk boundary
        let n2 = reader.read(&mut tmp).unwrap();
        assert_eq!(n2, 4);
        assert_eq!(&tmp[..n2], b"efgh");

        // read the rest to make sure we don't read over
        let n3 = reader.read(&mut tmp).unwrap();
        assert_eq!(n3, 3);
        assert_eq!(&tmp[..n3], b"ijk");

        // Reader exhausted — next read returns 0.
        let n4 = reader.read(&mut tmp).unwrap();
        assert_eq!(n4, 0);
    }

    #[test]
    #[allow(static_mut_refs)]
    fn test_envoy_buffers_reader_empty_chunk_skipped() {
        static mut CHUNK_BEFORE: [u8; 3] = *b"abc";
        static mut CHUNK_EMPTY: [u8; 0] = [];
        static mut CHUNK_AFTER: [u8; 3] = *b"xyz";
        let buffers = vec![
            EnvoyMutBuffer::new(unsafe { &mut CHUNK_BEFORE }),
            EnvoyMutBuffer::new(unsafe { &mut CHUNK_EMPTY }),
            EnvoyMutBuffer::new(unsafe { &mut CHUNK_AFTER }),
        ];
        let mut reader = EnvoyBuffersReader::new(buffers);
        let mut out = Vec::new();
        reader.read_to_end(&mut out).unwrap();
        assert_eq!(out, b"abcxyz");
    }

    // --- end EnvoyBuffersReader unit tests ---

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
        let filter_conf =
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
        let filter_conf =
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

    #[test]
    fn test_metadata_transformation() {
        let mut envoy_filter = envoy_proxy_dynamic_modules_rust_sdk::MockEnvoyHttpFilter::default();

        let json_str = r#"
        {
          "request": {
            "set": [
              { "name": "X-User", "value": "{{ header(\"x-user-id\") }}" }
            ],
            "dynamicMetadata": [
              { "namespace": "com.example.auth", "key": "user-id", "value": { "stringValue": "{{ header(\"x-user-id\") }}" } }
            ]
          }
        }
        "#;
        let filter_conf = FilterConfig::new(json_str)
            .unwrap_or_else(|| panic!("Failed to parse filter config json: {}", json_str));
        let mut filter = filter_conf.new_http_filter(&mut envoy_filter);

        envoy_filter
            .expect_get_most_specific_route_config()
            .returning(|| None);

        envoy_filter.expect_get_request_headers().returning(|| {
            vec![
                (EnvoyBuffer::new("host"), EnvoyBuffer::new("example.com")),
                (EnvoyBuffer::new("x-user-id"), EnvoyBuffer::new("alice")),
            ]
        });

        envoy_filter
            .expect_set_request_header()
            .times(1)
            .returning(|key, value: &[u8]| {
                assert_eq!(key, "X-User");
                assert_eq!(std::str::from_utf8(value).unwrap(), "alice");
                true
            });

        envoy_filter
            .expect_set_dynamic_metadata_string()
            .times(1)
            .returning(|namespace, key, value| {
                assert_eq!(namespace, "com.example.auth");
                assert_eq!(key, "user-id");
                assert_eq!(value, "alice");
            });

        assert_eq!(
            filter.on_request_headers(&mut envoy_filter, true),
            abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue
        );
    }

    /// Regression test: when a request body arrives in a single chunk,
    /// `get_buffered_request_body` returns None because no prior
    /// `StopIterationAndBuffer` populated it — the data sits in the
    /// "received" buffer only.  Without the fallback to
    /// `get_received_request_body`, `parse_request_json_body` returns Null
    /// and the undeclared-variables check fires a 400.
    #[test]
    fn test_json_body_extracted_from_received_when_buffered_is_empty() {
        let mut envoy_filter = envoy_proxy_dynamic_modules_rust_sdk::MockEnvoyHttpFilter::default();

        // Config: parse body as JSON, extract "model" field into X-Model header.
        let json_str = r#"
        {
          "request": {
            "body": { "parseAs": "AsJson" },
            "set": [
              { "name": "X-Model", "value": "{{ model }}" }
            ]
          }
        }
        "#;
        let filter_conf = FilterConfig::new(json_str).expect("Failed to parse filter config json");
        let mut filter = filter_conf.new_http_filter(&mut envoy_filter);

        // No per-route config — use the base config.
        envoy_filter
            .expect_get_most_specific_route_config()
            .returning(|| None);

        envoy_filter
            .expect_get_request_headers()
            .returning(|| vec![(EnvoyBuffer::new("host"), EnvoyBuffer::new("example.com"))]);

        // Simulate the single-chunk scenario:
        //   • buffered body is empty (None)
        //   • received body contains the JSON payload
        envoy_filter
            .expect_get_buffered_request_body()
            .returning(|| None);

        static mut BODY: [u8; 19] = *b"{\"model\":\"gpt-4\"}  ";
        envoy_filter
            .expect_get_received_request_body()
            .returning(|| Some(vec![EnvoyMutBuffer::new(unsafe { &mut BODY[..17] })]));

        // Expect the extracted header to be set with the model value.
        envoy_filter
            .expect_set_request_header()
            .times(1)
            .returning(|key, value: &[u8]| {
                assert_eq!(key, "X-Model");
                assert_eq!(std::str::from_utf8(value).unwrap(), "gpt-4");
                true
            });

        // Phase 1: headers arrive, body not yet received → buffer.
        assert_eq!(
            filter.on_request_headers(&mut envoy_filter, false),
            abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::StopIteration
        );

        // Phase 2: entire body arrives in one chunk (end_of_stream = true).
        // Without the fix this returns StopIterationAndBuffer (400 sent).
        // With the fix the body is found via received fallback → Continue.
        assert_eq!(
            filter.on_request_body(&mut envoy_filter, true),
            abi::envoy_dynamic_module_type_on_http_filter_request_body_status::Continue
        );
    }

    #[test]
    fn test_detect_upgrade_request() {
        fn h(pairs: &[(&str, &str)]) -> HashMap<String, String> {
            pairs
                .iter()
                .map(|(k, v)| (k.to_string(), v.to_string()))
                .collect()
        }

        // detect websocket upgrade regardless of casing
        assert!(Filter::detect_upgrade_request(&h(&[(
            "upgrade",
            "websocket"
        )])));
        assert!(Filter::detect_upgrade_request(&h(&[(
            "upgrade",
            "WEBSOCKET"
        )])));
        assert!(Filter::detect_upgrade_request(&h(&[(
            "upgrade",
            "WebSocket"
        )])));

        // detect CONNECT request regardless of casing
        assert!(Filter::detect_upgrade_request(&h(&[(
            ":method", "connect"
        )])));
        assert!(Filter::detect_upgrade_request(&h(&[(
            ":method", "CONNECT"
        )])));
        assert!(Filter::detect_upgrade_request(&h(&[(
            ":method", "Connect"
        )])));

        // no headers — no match
        assert!(!Filter::detect_upgrade_request(&h(&[])));

        // upgrade header with non-websocket value — no match
        assert!(!Filter::detect_upgrade_request(&h(&[("upgrade", "h2c")])));

        // :method with non-CONNECT value — no match
        assert!(!Filter::detect_upgrade_request(&h(&[(":method", "GET")])));
    }
}
