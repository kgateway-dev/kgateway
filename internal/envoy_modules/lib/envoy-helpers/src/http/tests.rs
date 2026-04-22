#![allow(clippy::unwrap_used, clippy::expect_used)]

use super::detect_upgrade_request;
use std::collections::HashMap;

fn headers(pairs: &[(&str, &str)]) -> HashMap<String, String> {
    pairs
        .iter()
        .map(|(k, v)| (k.to_string(), v.to_string()))
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
