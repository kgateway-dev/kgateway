#![allow(clippy::unwrap_used, clippy::expect_used)]

use super::*;

fn build(json: &str) -> Acl {
    Acl::from_json(json).unwrap()
}

fn ip(s: &str) -> IpAddr {
    s.parse().unwrap()
}

fn act(acl: &Acl, s: &str) -> Action {
    acl.evaluate(ip(s)).action
}

#[test]
fn default_allow_no_rules_allows_any() {
    let acl = build(r#"{"defaultAction":"allow","rules":[]}"#);
    assert_eq!(act(&acl, "1.2.3.4"), Action::Allow);
    assert_eq!(act(&acl, "2001:db8::1"), Action::Allow);
}

#[test]
fn default_deny_no_rules_denies_any() {
    let acl = build(r#"{"defaultAction":"deny","rules":[]}"#);
    assert_eq!(act(&acl, "1.2.3.4"), Action::Deny);
    assert_eq!(act(&acl, "2001:db8::1"), Action::Deny);
}

#[test]
fn default_deny_allow_subnet() {
    let acl = build(r#"{"defaultAction":"deny","rules":[{"cidr":"10.0.0.0/8","action":"allow"}]}"#);
    assert_eq!(act(&acl, "10.5.5.5"), Action::Allow);
    assert_eq!(act(&acl, "11.0.0.1"), Action::Deny);
}

#[test]
fn default_allow_deny_subnet() {
    let acl =
        build(r#"{"defaultAction":"allow","rules":[{"cidr":"192.168.0.0/16","action":"deny"}]}"#);
    assert_eq!(act(&acl, "192.168.1.1"), Action::Deny);
    assert_eq!(act(&acl, "8.8.8.8"), Action::Allow);
}

#[test]
fn hole_punch_allow_inside_deny() {
    let acl = build(
        r#"{"defaultAction":"allow","rules":[
            {"cidr":"10.0.0.0/8","action":"deny"},
            {"cidr":"10.1.0.0/16","action":"allow"}
        ]}"#,
    );
    assert_eq!(act(&acl, "10.1.2.3"), Action::Allow);
    assert_eq!(act(&acl, "10.2.0.1"), Action::Deny);
    assert_eq!(act(&acl, "11.0.0.1"), Action::Allow);
}

#[test]
fn deep_hole_punch() {
    let acl = build(
        r#"{"defaultAction":"allow","rules":[
            {"cidr":"10.0.0.0/8","action":"deny"},
            {"cidr":"10.1.0.0/16","action":"allow"},
            {"cidr":"10.1.2.3/32","action":"deny"}
        ]}"#,
    );
    assert_eq!(act(&acl, "10.1.2.3"), Action::Deny);
    assert_eq!(act(&acl, "10.1.2.4"), Action::Allow);
    assert_eq!(act(&acl, "10.2.0.1"), Action::Deny);
    assert_eq!(act(&acl, "11.0.0.1"), Action::Allow);
}

#[test]
fn bare_ip_rule_treated_as_single_host() {
    let acl = build(r#"{"defaultAction":"allow","rules":[{"cidr":"1.2.3.4","action":"deny"}]}"#);
    assert_eq!(act(&acl, "1.2.3.4"), Action::Deny);
    assert_eq!(act(&acl, "1.2.3.5"), Action::Allow);
}

#[test]
fn ipv6_allow_over_default_deny() {
    let acl =
        build(r#"{"defaultAction":"deny","rules":[{"cidr":"2001:db8::/32","action":"allow"}]}"#);
    assert_eq!(act(&acl, "2001:db8::1"), Action::Allow);
    assert_eq!(act(&acl, "2001:db8:1234:5678::1"), Action::Allow);
    assert_eq!(act(&acl, "2002::1"), Action::Deny);
}

#[test]
fn ipv4_mapped_ipv6_routes_to_v4_trie() {
    let acl = build(r#"{"defaultAction":"allow","rules":[{"cidr":"10.0.0.0/8","action":"deny"}]}"#);
    assert_eq!(act(&acl, "::ffff:10.1.1.1"), Action::Deny);
    assert_eq!(act(&acl, "::ffff:11.1.1.1"), Action::Allow);
}

#[test]
fn v4_rule_does_not_leak_to_v6_and_vice_versa() {
    let acl = build(
        r#"{"defaultAction":"allow","rules":[
            {"cidr":"10.0.0.0/8","action":"deny"},
            {"cidr":"2001:db8::/32","action":"deny"}
        ]}"#,
    );
    assert_eq!(act(&acl, "10.0.0.1"), Action::Deny);
    assert_eq!(act(&acl, "2001::1"), Action::Allow);
    assert_eq!(act(&acl, "2001:db8::5"), Action::Deny);
    assert_eq!(act(&acl, "20.0.0.1"), Action::Allow);
}

#[test]
fn malformed_cidr_rejected() {
    let res = Acl::from_json(
        r#"{"defaultAction":"allow","rules":[{"cidr":"not-a-cidr","action":"deny"}]}"#,
    );
    match res {
        Err(AclError::InvalidCidr(s)) => assert_eq!(s, "not-a-cidr"),
        Err(e) => panic!("expected InvalidCidr, got {e:?}"),
        Ok(_) => panic!("expected error"),
    }
}

#[test]
fn malformed_json_rejected() {
    let res = Acl::from_json("{not json");
    match res {
        Err(AclError::Json(_)) => {}
        Err(e) => panic!("expected Json error, got {e:?}"),
        Ok(_) => panic!("expected error"),
    }
}

#[test]
fn rule_order_does_not_matter() {
    let wide_first = build(
        r#"{"defaultAction":"allow","rules":[
            {"cidr":"10.0.0.0/8","action":"deny"},
            {"cidr":"10.1.0.0/16","action":"allow"}
        ]}"#,
    );
    let narrow_first = build(
        r#"{"defaultAction":"allow","rules":[
            {"cidr":"10.1.0.0/16","action":"allow"},
            {"cidr":"10.0.0.0/8","action":"deny"}
        ]}"#,
    );
    for addr in ["10.1.2.3", "10.2.0.1", "11.0.0.1"] {
        assert_eq!(act(&wide_first, addr), act(&narrow_first, addr));
    }
}

#[test]
fn last_rule_win_for_duplicated_cidr() {
    let deny_first = build(
        r#"{"defaultAction":"allow","rules":[
            {"cidr":"10.0.0.0/8","action":"deny"},
            {"cidr":"10.0.0.0/8","action":"allow"}
        ]}"#,
    );
    let allow_first = build(
        r#"{"defaultAction":"allow","rules":[
            {"cidr":"10.0.0.0/8","action":"allow"},
            {"cidr":"10.0.0.0/8","action":"deny"}
        ]}"#,
    );
    assert_eq!(act(&deny_first, "10.0.0.1"), Action::Allow);
    assert_eq!(act(&allow_first, "10.0.0.1"), Action::Deny);
}

#[test]
fn zero_prefix_overrides_default() {
    let acl = build(
        r#"{"defaultAction":"allow","rules":[
            {"cidr":"0.0.0.0/0","action":"deny"},
            {"cidr":"1.1.1.1/32","action":"allow"}
        ]}"#,
    );
    assert_eq!(act(&acl, "1.1.1.1"), Action::Allow);
    assert_eq!(act(&acl, "2.2.2.2"), Action::Deny);
    assert_eq!(act(&acl, "8.8.8.8"), Action::Deny);
}

#[test]
fn matched_rule_name_returned_when_rule_named() {
    let acl = build(
        r#"{"defaultAction":"allow","rules":[
            {"name":"block-internal","cidr":"10.0.0.0/8","action":"deny"}
        ]}"#,
    );
    let d = acl.evaluate(ip("10.1.2.3"));
    assert_eq!(d.action, Action::Deny);
    assert_eq!(d.matched_rule_name, Some("block-internal"));
    assert!(!d.default_applied);
}

#[test]
fn matched_rule_name_none_when_rule_unnamed() {
    let acl = build(r#"{"defaultAction":"allow","rules":[{"cidr":"10.0.0.0/8","action":"deny"}]}"#);
    let d = acl.evaluate(ip("10.1.2.3"));
    assert_eq!(d.action, Action::Deny);
    assert_eq!(d.matched_rule_name, None);
    assert!(!d.default_applied);
}

#[test]
fn default_applied_flag_set_when_no_rule_matches() {
    let acl = build(
        r#"{"defaultAction":"deny","rules":[
            {"name":"allow-lan","cidr":"10.0.0.0/8","action":"allow"}
        ]}"#,
    );
    let d = acl.evaluate(ip("8.8.8.8"));
    assert_eq!(d.action, Action::Deny);
    assert_eq!(d.matched_rule_name, None);
    assert!(d.default_applied);
}

#[test]
fn default_applied_flag_also_set_for_default_allow() {
    let acl = build(r#"{"defaultAction":"allow","rules":[]}"#);
    let d = acl.evaluate(ip("1.2.3.4"));
    assert_eq!(d.action, Action::Allow);
    assert!(d.default_applied);
    assert_eq!(d.matched_rule_name, None);
}

#[test]
fn matched_rule_name_reports_most_specific_rule() {
    let acl = build(
        r#"{"defaultAction":"allow","rules":[
            {"name":"wide-deny","cidr":"10.0.0.0/8","action":"deny"},
            {"name":"narrow-allow","cidr":"10.1.0.0/16","action":"allow"},
            {"name":"single-host-deny","cidr":"10.1.2.3/32","action":"deny"}
        ]}"#,
    );
    for (addr, expected) in [
        ("10.1.2.3", "single-host-deny"),
        ("10.1.2.4", "narrow-allow"),
        ("10.2.0.1", "wide-deny"),
    ] {
        let d = acl.evaluate(ip(addr));
        assert_eq!(d.matched_rule_name, Some(expected));
        assert!(!d.default_applied);
    }
}

#[test]
fn deny_response_defaults_when_absent() {
    let acl = build(r#"{"defaultAction":"deny"}"#);
    let dr = acl.deny_response();
    assert_eq!(dr.status_code, 403);
    assert!(dr.headers.is_empty());
}

#[test]
fn deny_response_custom_status_code() {
    let acl = build(r#"{"defaultAction":"deny","denyResponse":{"statusCode":401},"rules":[]}"#);
    assert_eq!(acl.deny_response().status_code, 401);
    assert!(acl.deny_response().headers.is_empty());
}

#[test]
fn deny_response_custom_headers() {
    let acl = build(
        r#"{"defaultAction":"deny","denyResponse":{
            "statusCode":451,
            "headers":[
                {"name":"X-Blocked-Reason","value":"geo-policy"},
                {"name":"Retry-After","value":"3600"}
            ]
        },"rules":[]}"#,
    );
    let dr = acl.deny_response();
    assert_eq!(dr.status_code, 451);
    assert_eq!(dr.headers.len(), 2);
    assert_eq!(dr.headers[0].name, "X-Blocked-Reason");
    assert_eq!(dr.headers[0].value, "geo-policy");
    assert_eq!(dr.headers[1].name, "Retry-After");
    assert_eq!(dr.headers[1].value, "3600");
}

#[test]
fn deny_response_status_code_only_defaults_headers() {
    let acl = build(r#"{"defaultAction":"deny","denyResponse":{"statusCode":429},"rules":[]}"#);
    let dr = acl.deny_response();
    assert_eq!(dr.status_code, 429);
    assert!(dr.headers.is_empty());
}

#[test]
fn deny_response_headers_only_defaults_status() {
    let acl = build(
        r#"{"defaultAction":"deny","denyResponse":{
            "headers":[{"name":"X-Foo","value":"bar"}]
        },"rules":[]}"#,
    );
    let dr = acl.deny_response();
    assert_eq!(dr.status_code, 403);
    assert_eq!(dr.headers.len(), 1);
}

#[test]
fn deny_response_add_blocked_by_header_defaults_to_none() {
    let acl = build(r#"{"defaultAction":"deny"}"#);
    assert!(acl.deny_response().add_blocked_by_header.is_none());
}

#[test]
fn deny_response_add_blocked_by_header_parses() {
    let acl = build(
        r#"{"defaultAction":"deny","denyResponse":{
            "addBlockedByHeader":"X-Blocked-By"
        },"rules":[]}"#,
    );
    assert_eq!(
        acl.deny_response().add_blocked_by_header.as_deref(),
        Some("X-Blocked-By")
    );
}
