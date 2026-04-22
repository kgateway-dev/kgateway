#![allow(clippy::unwrap_used, clippy::expect_used)]

use super::*;
use mockall::*;

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
