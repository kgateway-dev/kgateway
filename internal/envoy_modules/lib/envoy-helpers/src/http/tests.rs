#![allow(clippy::unwrap_used, clippy::expect_used)]

use super::{
    create_headers_map, detect_upgrade_request, parse_cookie_maps, parse_cookie_string,
    parse_cookies_from_header_map,
};
use envoy_proxy_dynamic_modules_rust_sdk::EnvoyBuffer;
use std::collections::HashMap;

fn make_headers(
    pairs: &[(&'static str, &'static str)],
) -> Vec<(EnvoyBuffer<'static>, EnvoyBuffer<'static>)> {
    pairs
        .iter()
        .map(|(k, v)| {
            (
                EnvoyBuffer::new(k.as_bytes()),
                EnvoyBuffer::new(v.as_bytes()),
            )
        })
        .collect()
}

fn headers(pairs: &[(&str, &str)]) -> HashMap<String, Vec<String>> {
    pairs
        .iter()
        .map(|(k, v)| (k.to_string(), vec![v.to_string()]))
        .collect()
}

#[test]
fn test_detect_upgrade_websocket_lowercase() {
    let h = headers(&[("upgrade", "websocket")]);
    assert!(detect_upgrade_request(&h));
}

#[test]
fn test_detect_upgrade_websocket_mixed_case() {
    let h = headers(&[("upgrade", "WebSocket")]);
    assert!(detect_upgrade_request(&h));
}

#[test]
fn test_detect_upgrade_websocket_uppercase() {
    let h = headers(&[("upgrade", "WEBSOCKET")]);
    assert!(detect_upgrade_request(&h));
}

#[test]
fn test_detect_upgrade_connect_uppercase() {
    let h = headers(&[(":method", "CONNECT")]);
    assert!(detect_upgrade_request(&h));
}

#[test]
fn test_detect_upgrade_connect_lowercase() {
    let h = headers(&[(":method", "connect")]);
    assert!(detect_upgrade_request(&h));
}

#[test]
fn test_detect_upgrade_regular_get() {
    let h = headers(&[(":method", "GET"), (":path", "/foo")]);
    assert!(!detect_upgrade_request(&h));
}

#[test]
fn test_detect_upgrade_empty_headers() {
    let h = HashMap::new();
    assert!(!detect_upgrade_request(&h));
}

#[test]
fn test_detect_upgrade_non_websocket_upgrade() {
    // An upgrade header present but not for websocket should not match.
    let h = headers(&[("upgrade", "h2c")]);
    assert!(!detect_upgrade_request(&h));
}

#[test]
fn test_detect_upgrade_post_with_upgrade_header() {
    // POST with a websocket upgrade header should still be detected as upgrade.
    let h = headers(&[(":method", "POST"), ("upgrade", "websocket")]);
    assert!(detect_upgrade_request(&h));
}

#[test]
fn test_parse_cookie_string_basic() {
    let cookies = parse_cookie_string("session=abc123; user=john doe");
    assert_eq!(cookies.get("session"), Some(&"abc123".to_string()));
    assert_eq!(cookies.get("user"), Some(&"john doe".to_string()));
}

#[test]
fn test_parse_cookie_string_case_sensitive_keys() {
    // Cookie names are case-sensitive per RFC 6265.
    let cookies = parse_cookie_string("Session=abc123; USER=johndoe");
    assert_eq!(cookies.get("Session"), Some(&"abc123".to_string()));
    assert_eq!(cookies.get("USER"), Some(&"johndoe".to_string()));
    assert_eq!(cookies.get("session"), None);
    assert_eq!(cookies.get("user"), None);
}

#[test]
fn test_parse_cookie_string_comma_not_a_separator() {
    // Commas are no longer treated as separators — each Cookie header value
    // is a separate Vec element at the call site, so a comma here is part of
    // a cookie value, not a delimiter between cookies.
    let cookies = parse_cookie_string("session=abc123");
    assert_eq!(cookies.get("session"), Some(&"abc123".to_string()));
    // A comma inside a value is preserved as-is
    let cookies2 = parse_cookie_string("token=abc,def");
    assert_eq!(cookies2.get("token"), Some(&"abc,def".to_string()));
}

#[test]
fn test_parse_cookie_string_value_with_equals() {
    let cookies = parse_cookie_string("token=abc=def=ghi");
    assert_eq!(cookies.get("token"), Some(&"abc=def=ghi".to_string()));
}

#[test]
fn test_parse_cookie_string_duplicate_keeps_first() {
    let cookies = parse_cookie_string("session=first; session=second");
    assert_eq!(cookies.get("session"), Some(&"first".to_string()));
}

#[test]
fn test_parse_cookie_string_empty() {
    let cookies = parse_cookie_string("");
    assert!(cookies.is_empty());
}

#[test]
fn test_parse_cookie_string_no_equals_skipped() {
    let cookies = parse_cookie_string("novalue; session=abc");
    assert!(!cookies.contains_key("novalue"));
    assert_eq!(cookies.get("session"), Some(&"abc".to_string()));
}

#[test]
fn test_parse_cookie_string_whitespace_trimmed() {
    let cookies = parse_cookie_string(" session = abc123 ; user = johndoe ");
    assert_eq!(cookies.get("session"), Some(&"abc123".to_string()));
    assert_eq!(cookies.get("user"), Some(&"johndoe".to_string()));
}

#[test]
fn test_parse_cookies_from_header_map_missing_cookie() {
    let headers: HashMap<String, Vec<String>> = HashMap::new();
    let cookies = parse_cookies_from_header_map(&headers, false);
    assert!(cookies.is_empty());
}

#[test]
fn test_parse_cookies_from_header_map_with_cookie() {
    let mut headers: HashMap<String, Vec<String>> = HashMap::new();
    headers.insert(
        "cookie".to_string(),
        vec!["session=abc; user=johndoe".to_string()],
    );
    let cookies = parse_cookies_from_header_map(&headers, false);
    assert_eq!(cookies.get("session"), Some(&"abc".to_string()));
    assert_eq!(cookies.get("user"), Some(&"johndoe".to_string()));
}

#[test]
fn test_parse_cookies_from_header_map_multiple_cookie_headers() {
    // Simulates two separate Cookie headers arriving from Envoy as distinct Vec elements.
    let mut headers: HashMap<String, Vec<String>> = HashMap::new();
    headers.insert(
        "cookie".to_string(),
        vec!["session=abc".to_string(), "user=johndoe".to_string()],
    );
    let cookies = parse_cookies_from_header_map(&headers, false);
    assert_eq!(cookies.get("session"), Some(&"abc".to_string()));
    assert_eq!(cookies.get("user"), Some(&"johndoe".to_string()));
}

#[test]
fn test_parse_cookies_from_header_map_case_insensitive() {
    let mut headers: HashMap<String, Vec<String>> = HashMap::new();
    headers.insert(
        "cookie".to_string(),
        vec!["Session=abc; USER=johndoe".to_string()],
    );
    let cookies = parse_cookies_from_header_map(&headers, true);
    // Keys normalized to lowercase
    assert_eq!(cookies.get("session"), Some(&"abc".to_string()));
    assert_eq!(cookies.get("user"), Some(&"johndoe".to_string()));
    assert_eq!(cookies.get("Session"), None);
}

#[test]
fn test_parse_cookie_maps_both_none_when_neither_needed() {
    let mut headers: HashMap<String, Vec<String>> = HashMap::new();
    headers.insert("cookie".to_string(), vec!["session=abc".to_string()]);
    let (exact, insensitive) = parse_cookie_maps(&headers, false, false);
    assert!(exact.is_none());
    assert!(insensitive.is_none());
}

#[test]
fn test_parse_cookie_maps_exact_only() {
    let mut headers: HashMap<String, Vec<String>> = HashMap::new();
    headers.insert("cookie".to_string(), vec!["Session=abc".to_string()]);
    let (exact, insensitive) = parse_cookie_maps(&headers, true, false);
    let exact = exact.unwrap();
    assert_eq!(exact.get("Session"), Some(&"abc".to_string()));
    assert_eq!(exact.get("session"), None);
    assert!(insensitive.is_none());
}

#[test]
fn test_parse_cookie_maps_insensitive_only() {
    let mut headers: HashMap<String, Vec<String>> = HashMap::new();
    headers.insert("cookie".to_string(), vec!["Session=abc".to_string()]);
    let (exact, insensitive) = parse_cookie_maps(&headers, false, true);
    assert!(exact.is_none());
    let insensitive = insensitive.unwrap();
    assert_eq!(insensitive.get("session"), Some(&"abc".to_string()));
    assert_eq!(insensitive.get("Session"), None);
}

#[test]
fn test_parse_cookie_maps_both_single_pass() {
    // Both maps are built in one header iteration.
    let mut headers: HashMap<String, Vec<String>> = HashMap::new();
    headers.insert(
        "cookie".to_string(),
        vec!["Session=abc123".to_string(), "User=johndoe".to_string()],
    );
    let (exact, insensitive) = parse_cookie_maps(&headers, true, true);
    let exact = exact.unwrap();
    let insensitive = insensitive.unwrap();
    // exact map preserves original case
    assert_eq!(exact.get("Session"), Some(&"abc123".to_string()));
    assert_eq!(exact.get("User"), Some(&"johndoe".to_string()));
    assert_eq!(exact.get("session"), None);
    // insensitive map lowercases keys
    assert_eq!(insensitive.get("session"), Some(&"abc123".to_string()));
    assert_eq!(insensitive.get("user"), Some(&"johndoe".to_string()));
    assert_eq!(insensitive.get("Session"), None);
}

#[test]
fn test_create_headers_map_collected_as_vec_with_case_insensitive_key() {
    let m = create_headers_map(make_headers(&[
        ("cookie", "session=abc"),
        ("Cookie", "user=johndoe"),
        ("content-Type", "application/json"),
        ("x-Foo", "bar"),
    ]));
    assert_eq!(
        m.get("cookie"),
        Some(&vec!["session=abc".to_string(), "user=johndoe".to_string()])
    );
    assert_eq!(
        m.get("content-type"),
        Some(&vec!["application/json".to_string()])
    );
    assert_eq!(m.get("x-foo"), Some(&vec!["bar".to_string()]));
}
