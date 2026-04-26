#![deny(clippy::unwrap_used, clippy::expect_used)]

use envoy_proxy_dynamic_modules_rust_sdk::EnvoyBuffer;
use std::collections::HashMap;

/// Converts a list of raw Envoy header pairs into a [`HashMap`].
/// Header pairs with non-UTF-8 keys or values are silently dropped.
pub fn create_headers_map(headers: Vec<(EnvoyBuffer, EnvoyBuffer)>) -> HashMap<String, String> {
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

/// Returns true if the request is a WebSocket upgrade or an HTTP CONNECT request.
pub fn detect_upgrade_request(headers: &HashMap<String, String>) -> bool {
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

#[cfg(test)]
mod tests;
