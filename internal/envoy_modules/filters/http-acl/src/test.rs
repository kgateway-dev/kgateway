#![allow(clippy::unwrap_used, clippy::expect_used)]

use super::*;
use envoy_proxy_dynamic_modules_rust_sdk::{EnvoyBuffer, MockEnvoyHttpFilter};
use std::any::Any;
use std::sync::Arc;

// Test-only helper: build a FilterConfig without going through an
// EnvoyHttpFilterConfig (which has no automock in the SDK and no public way to
// construct an EnvoyCounterId).
//
// SAFETY: EnvoyCounterId is a public tuple struct wrapping a single `usize`
// (`pub struct EnvoyCounterId(usize)`), and `0` is a valid value. In tests
// the id never reaches Envoy — every `increment_counter` call is intercepted by
// the mock.
fn make_filter_config(cfg: &str) -> FilterConfig {
    let acl = Acl::from_json(cfg).expect("valid test config");
    FilterConfig {
        acl: Arc::new(acl),
        blocked_counter: unsafe { std::mem::zeroed() },
    }
}

fn mock_with_source(addr: Option<&'static str>) -> MockEnvoyHttpFilter {
    let mut mock = MockEnvoyHttpFilter::default();
    mock.expect_get_most_specific_route_config()
        .returning(|| None);
    match addr {
        Some(s) => {
            mock.expect_get_attribute_string()
                .returning(move |_| Some(EnvoyBuffer::new(s)));
        }
        None => {
            mock.expect_get_attribute_string().returning(|_| None);
        }
    }
    mock
}

fn expect_counter_incremented_once(mock: &mut MockEnvoyHttpFilter) {
    mock.expect_increment_counter()
        .withf(|_, value| *value == 1)
        .times(1)
        .returning(|_, _| Ok(()));
}

fn expect_metadata(mock: &mut MockEnvoyHttpFilter, expected_value: &'static str) {
    mock.expect_set_dynamic_metadata_string()
        .withf(move |ns, key, val| {
            ns == "dev.kgateway.http.acl" && key == "blocked-by" && val == expected_value
        })
        .times(1)
        .returning(|_, _, _| ());
}

fn run(filter_cfg: FilterConfig, mock: &mut MockEnvoyHttpFilter) -> u32 {
    let mut filter = filter_cfg.new_http_filter(mock);
    let status = filter.on_request_headers(mock, true);
    status as u32
}

fn continue_status() -> u32 {
    abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue as u32
}

fn stop_status() -> u32 {
    abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::StopIteration as u32
}

// ---- allow ----

#[test]
fn allow_decision_returns_continue_with_no_side_effects() {
    let cfg = make_filter_config(r#"{"defaultAction":"allow"}"#);
    let mut mock = mock_with_source(Some("10.1.2.3"));
    // No expectations on send_response / set_dynamic_metadata_string / increment_counter.
    // mockall fails the test if any are called unexpectedly.
    assert_eq!(run(cfg, &mut mock), continue_status());
}

#[test]
fn allow_via_rule_does_not_increment_counter() {
    let cfg = make_filter_config(
        r#"{"defaultAction":"deny","rules":[{"cidr":"10.0.0.0/8","action":"allow"}]}"#,
    );
    let mut mock = mock_with_source(Some("10.5.5.5"));
    assert_eq!(run(cfg, &mut mock), continue_status());
}

// ---- deny: metadata + counter + response ----

#[test]
fn deny_by_named_rule_emits_rule_name_metadata() {
    let cfg = make_filter_config(
        r#"{
            "defaultAction":"allow",
            "rules":[{"name":"block-internal","cidr":"10.0.0.0/8","action":"deny"}]
        }"#,
    );
    let mut mock = mock_with_source(Some("10.5.5.5"));
    expect_metadata(&mut mock, "block-internal");
    expect_counter_incremented_once(&mut mock);
    mock.expect_send_response()
        .times(1)
        .returning(|status, headers, body, _| {
            assert_eq!(status, 403);
            assert!(body.is_none());
            // No addBlockedByHeader configured → no extra headers.
            assert!(headers.is_empty());
        });

    assert_eq!(run(cfg, &mut mock), stop_status());
}

#[test]
fn deny_by_unnamed_rule_emits_rule_literal_metadata() {
    let cfg = make_filter_config(
        r#"{"defaultAction":"allow","rules":[{"cidr":"10.0.0.0/8","action":"deny"}]}"#,
    );
    let mut mock = mock_with_source(Some("10.5.5.5"));
    expect_metadata(&mut mock, "rule");
    expect_counter_incremented_once(&mut mock);
    mock.expect_send_response()
        .times(1)
        .returning(|_, _, _, _| ());

    assert_eq!(run(cfg, &mut mock), stop_status());
}

#[test]
fn deny_by_default_action_emits_default_metadata() {
    let cfg = make_filter_config(r#"{"defaultAction":"deny"}"#);
    let mut mock = mock_with_source(Some("10.5.5.5"));
    expect_metadata(&mut mock, "default");
    expect_counter_incremented_once(&mut mock);
    mock.expect_send_response()
        .times(1)
        .returning(|_, _, _, _| ());

    assert_eq!(run(cfg, &mut mock), stop_status());
}

#[test]
fn unknown_source_ip_denies_with_unknown_ip_metadata() {
    let cfg = make_filter_config(r#"{"defaultAction":"allow"}"#);
    let mut mock = mock_with_source(None); // attribute returns None
    expect_metadata(&mut mock, "unknown-ip");
    expect_counter_incremented_once(&mut mock);
    mock.expect_send_response()
        .times(1)
        .returning(|_, _, _, _| ());

    assert_eq!(run(cfg, &mut mock), stop_status());
}

// ---- deny response customization ----

#[test]
fn deny_uses_custom_status_code_and_headers() {
    let cfg = make_filter_config(
        r#"{
            "defaultAction":"deny",
            "denyResponse":{
                "statusCode":451,
                "headers":[
                    {"name":"X-Blocked-Reason","value":"geo-policy"},
                    {"name":"Retry-After","value":"3600"}
                ]
            }
        }"#,
    );
    let mut mock = mock_with_source(Some("8.8.8.8"));
    expect_metadata(&mut mock, "default");
    expect_counter_incremented_once(&mut mock);
    mock.expect_send_response()
        .times(1)
        .returning(|status, headers, body, _| {
            assert_eq!(status, 451);
            assert!(body.is_none());
            let seen: Vec<(&str, &[u8])> = headers.into_iter().collect();
            assert_eq!(
                seen,
                vec![
                    ("X-Blocked-Reason", b"geo-policy" as &[u8]),
                    ("Retry-After", b"3600" as &[u8]),
                ]
            );
        });

    assert_eq!(run(cfg, &mut mock), stop_status());
}

#[test]
fn add_blocked_by_header_appended_with_rule_name() {
    let cfg = make_filter_config(
        r#"{
            "defaultAction":"allow",
            "denyResponse":{"addBlockedByHeader":"X-Blocked-By"},
            "rules":[{"name":"block-internal","cidr":"10.0.0.0/8","action":"deny"}]
        }"#,
    );
    let mut mock = mock_with_source(Some("10.5.5.5"));
    expect_metadata(&mut mock, "block-internal");
    expect_counter_incremented_once(&mut mock);
    mock.expect_send_response()
        .times(1)
        .returning(|status, headers, _, _| {
            assert_eq!(status, 403);
            let seen: Vec<(&str, &[u8])> = headers.into_iter().collect();
            assert_eq!(seen, vec![("X-Blocked-By", b"block-internal" as &[u8])]);
        });

    assert_eq!(run(cfg, &mut mock), stop_status());
}

#[test]
fn add_blocked_by_header_carries_rule_literal_for_unnamed_rule() {
    let cfg = make_filter_config(
        r#"{
            "defaultAction":"allow",
            "denyResponse":{"addBlockedByHeader":"X-Blocked-By"},
            "rules":[{"cidr":"10.0.0.0/8","action":"deny"}]
        }"#,
    );
    let mut mock = mock_with_source(Some("10.5.5.5"));
    expect_metadata(&mut mock, "rule");
    expect_counter_incremented_once(&mut mock);
    mock.expect_send_response()
        .times(1)
        .returning(|_, headers, _, _| {
            let seen: Vec<(&str, &[u8])> = headers.into_iter().collect();
            assert_eq!(seen, vec![("X-Blocked-By", b"rule" as &[u8])]);
        });

    assert_eq!(run(cfg, &mut mock), stop_status());
}

#[test]
fn add_blocked_by_header_carries_default_for_default_deny() {
    let cfg = make_filter_config(
        r#"{
            "defaultAction":"deny",
            "denyResponse":{"addBlockedByHeader":"X-Blocked-By"}
        }"#,
    );
    let mut mock = mock_with_source(Some("8.8.8.8"));
    expect_metadata(&mut mock, "default");
    expect_counter_incremented_once(&mut mock);
    mock.expect_send_response()
        .times(1)
        .returning(|_, headers, _, _| {
            let seen: Vec<(&str, &[u8])> = headers.into_iter().collect();
            assert_eq!(seen, vec![("X-Blocked-By", b"default" as &[u8])]);
        });

    assert_eq!(run(cfg, &mut mock), stop_status());
}

// ---- per-route ----

#[test]
fn per_route_config_overrides_filter_level_decision() {
    // Filter-level: allow-all. Per-route: deny-all.
    let cfg = make_filter_config(r#"{"defaultAction":"allow"}"#);
    let per_route_acl =
        Acl::from_json(r#"{"defaultAction":"deny"}"#).expect("valid per-route config");
    let prc: Arc<dyn Any> = Arc::new(PerRouteConfig {
        acl: Arc::new(per_route_acl),
    });

    let mut mock = MockEnvoyHttpFilter::default();
    mock.expect_get_most_specific_route_config()
        .returning_st(move || Some(Arc::clone(&prc)));
    mock.expect_get_attribute_string()
        .returning(|_| Some(EnvoyBuffer::new("8.8.8.8")));
    expect_metadata(&mut mock, "default");
    expect_counter_incremented_once(&mut mock);
    mock.expect_send_response()
        .times(1)
        .returning(|_, _, _, _| ());

    // Default would have allowed; per-route deny must apply.
    assert_eq!(run(cfg, &mut mock), stop_status());
}

#[test]
fn per_route_config_none_falls_back_to_filter_level() {
    // Filter-level: allow. Per-route absent → filter-level decides.
    let cfg = make_filter_config(r#"{"defaultAction":"allow"}"#);
    let mut mock = MockEnvoyHttpFilter::default();
    mock.expect_get_most_specific_route_config()
        .returning(|| None);
    mock.expect_get_attribute_string()
        .returning(|_| Some(EnvoyBuffer::new("8.8.8.8")));

    assert_eq!(run(cfg, &mut mock), continue_status());
}

#[test]
fn per_route_config_wrong_type_logs_and_falls_back() {
    // Filter-level: allow. Per-route config is the wrong type → filter falls back
    // to filter-level allow.
    let cfg = make_filter_config(r#"{"defaultAction":"allow"}"#);
    let bogus: Arc<dyn Any> = Arc::new(42u32);

    let mut mock = MockEnvoyHttpFilter::default();
    mock.expect_get_most_specific_route_config()
        .returning_st(move || Some(Arc::clone(&bogus)));
    mock.expect_get_attribute_string()
        .returning(|_| Some(EnvoyBuffer::new("8.8.8.8")));

    assert_eq!(run(cfg, &mut mock), continue_status());
}
