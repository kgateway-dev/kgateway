#![deny(clippy::unwrap_used, clippy::expect_used)]

use envoy_proxy_dynamic_modules_rust_sdk::EnvoyBuffer;
use std::collections::HashMap;

/// Converts a list of raw Envoy header pairs into a [`HashMap`].
/// Keys are normalized to lowercase. Header pairs with non-UTF-8 keys or
/// values are silently dropped. When the same header key appears more than
/// once, each value is pushed into the Vec for that key, preserving all
/// occurrences individually.
pub fn create_headers_map(
    headers: Vec<(EnvoyBuffer, EnvoyBuffer)>,
) -> HashMap<String, Vec<String>> {
    let mut headers_map: HashMap<String, Vec<String>> = HashMap::new();
    for (key, val) in headers {
        let Some(key) = std::str::from_utf8(key.as_slice()).ok() else {
            continue;
        };
        let Some(value) = std::str::from_utf8(val.as_slice()).ok() else {
            continue;
        };

        headers_map
            .entry(key.to_lowercase())
            .or_default()
            .push(value.to_string());
    }

    headers_map
}

/// Look up a header by name, lowercasing the key before the lookup so callers
/// don't have to remember to normalize case themselves.
pub fn get_header<'a>(
    headers: &'a HashMap<String, Vec<String>>,
    key: &str,
) -> Option<&'a Vec<String>> {
    headers.get(&key.to_lowercase())
}

/// Returns true if the request is a WebSocket upgrade or an HTTP CONNECT request.
pub fn detect_upgrade_request(headers: &HashMap<String, Vec<String>>) -> bool {
    if get_header(headers, "upgrade")
        .is_some_and(|v| v.iter().any(|s| s.eq_ignore_ascii_case("websocket")))
    {
        return true;
    }
    if get_header(headers, ":method")
        .and_then(|v| v.first())
        .is_some_and(|v| v.eq_ignore_ascii_case("connect"))
    {
        return true;
    }
    false
}

/// Parse a single Cookie header value into a name -> value map.
/// Cookie names are case-sensitive per RFC 6265. Cookies within the value are
/// separated by ';'. For duplicate names the first occurrence wins. Cookie
/// values may contain '=' characters; only the first '=' in each segment is
/// used as the name/value delimiter.
pub fn parse_cookie_string(cookie_str: &str) -> HashMap<String, String> {
    let mut cookies = HashMap::new();
    for segment in cookie_str.split(';') {
        let pair = segment.trim();
        if let Some(eq_pos) = pair.find('=') {
            let name = pair[..eq_pos].trim().to_string();
            let value = pair[eq_pos + 1..].trim().to_string();
            if !name.is_empty() {
                cookies.entry(name).or_insert(value);
            }
        }
    }
    cookies
}

/// The `(exact_map, insensitive_map)` pair returned by [`parse_cookie_maps`].
type CookieMaps = (
    Option<HashMap<String, String>>,
    Option<HashMap<String, String>>,
);

/// Parse all Cookie header values from a header map into up to two maps in a
/// single pass over the headers.
///
/// - `need_exact`: build a map with original-case keys for `get_cookie()`.
/// - `need_insensitive`: build a map with lowercase keys for `get_cookie_i()`.
///
/// When both flags are true the headers are iterated only once. Returns
/// `(exact_map, insensitive_map)` where each is `None` if its flag was false.
pub fn parse_cookie_maps(
    headers: &HashMap<String, Vec<String>>,
    need_exact: bool,
    need_insensitive: bool,
) -> CookieMaps {
    if !need_exact && !need_insensitive {
        return (None, None);
    }
    let mut exact = need_exact.then(HashMap::new);
    let mut insensitive = need_insensitive.then(HashMap::new);
    if let Some(cookie_values) = get_header(headers, "cookie") {
        // Derive mutable references once, outside the inner loop. Their
        // borrows of `exact` and `insensitive` are released when they go out
        // of scope at the end of this block, before the final move.
        let mut exact_m = exact.as_mut();
        let mut insensitive_m = insensitive.as_mut();
        for cookie_str in cookie_values {
            for (name, value) in parse_cookie_string(cookie_str) {
                if let Some(ref mut m) = insensitive_m {
                    m.entry(name.to_lowercase())
                        .or_insert_with(|| value.clone());
                }
                if let Some(ref mut m) = exact_m {
                    m.entry(name).or_insert(value);
                }
            }
        }
    }
    (exact, insensitive)
}

/// Convenience wrapper around [`parse_cookie_maps`] for callers that need only
/// one map. When `case_insensitive` is false the returned map has original-case
/// keys; when true the keys are lowercase.
pub fn parse_cookies_from_header_map(
    headers: &HashMap<String, Vec<String>>,
    case_insensitive: bool,
) -> HashMap<String, String> {
    let (exact, insensitive) = parse_cookie_maps(headers, !case_insensitive, case_insensitive);
    exact.or(insensitive).unwrap_or_default()
}

#[cfg(test)]
mod tests;
