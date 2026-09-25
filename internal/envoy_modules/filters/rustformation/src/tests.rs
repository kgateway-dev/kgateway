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
            (EnvoyBuffer::new(b"host"), EnvoyBuffer::new(b"example.com")),
            (
                EnvoyBuffer::new(b"x-donor"),
                EnvoyBuffer::new(b"thedonorvalue"),
            ),
        ]
    });

    envoy_filter.expect_get_response_headers().returning(|| {
        vec![
            (EnvoyBuffer::new(b"host"), EnvoyBuffer::new(b"example.com")),
            (
                EnvoyBuffer::new(b"x-donor"),
                EnvoyBuffer::new(b"thedonorvalue"),
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
            (EnvoyBuffer::new(b"host"), EnvoyBuffer::new(b"example.com")),
            (
                EnvoyBuffer::new(b"x-donor"),
                EnvoyBuffer::new(b"thedonorvalue"),
            ),
        ]
    });

    envoy_filter.expect_get_response_headers().returning(|| {
        vec![
            (EnvoyBuffer::new(b"host"), EnvoyBuffer::new(b"example.com")),
            (
                EnvoyBuffer::new(b"x-donor"),
                EnvoyBuffer::new(b"thedonorvalue"),
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
            (EnvoyBuffer::new(b"host"), EnvoyBuffer::new(b"example.com")),
            (EnvoyBuffer::new(b"x-user-id"), EnvoyBuffer::new(b"alice")),
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
#[allow(static_mut_refs)]
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
        .returning(|| vec![(EnvoyBuffer::new(b"host"), EnvoyBuffer::new(b"example.com"))]);

    // Simulate the single-chunk scenario:
    //   • buffered body is empty (None)
    //   • received body contains the JSON payload
    envoy_filter
        .expect_get_buffered_request_body()
        .returning(|| None);

    static mut BODY: [u8; 19] = *b"{\"model\":\"gpt-4\"}  ";
    envoy_filter
        .expect_get_received_request_body()
        .returning(|| Some(unsafe { vec![EnvoyMutBuffer::new(&mut BODY[..17])] }));

    // Expect the extracted header to be set with the model value.
    envoy_filter
        .expect_set_request_header()
        .times(1)
        .returning(|key, value: &[u8]| {
            assert_eq!(key, "X-Model");
            assert_eq!(std::str::from_utf8(value).unwrap(), "gpt-4");
            true
        });

    // Phase 1: headers arrive, body not yet received: buffer.
    assert_eq!(
        filter.on_request_headers(&mut envoy_filter, false),
        abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::StopIteration
    );

    // Phase 2: entire body arrives in one chunk (end_of_stream = true).
    // Without the fix this returns StopIterationAndBuffer (400 sent).
    // With the fix the body is found via received fallback: Continue.
    assert_eq!(
        filter.on_request_body(&mut envoy_filter, true),
        abi::envoy_dynamic_module_type_on_http_filter_request_body_status::Continue
    );
}

#[test]
fn test_selective_model_header_matches_full_transform() {
    use std::sync::{Arc, Mutex};

    for body in [
        br#"{"model":"first","messages":[{"model":"nested","content":"hello"}],"model":"last"}"#
            .as_slice(),
        br#"{"messages":[{"model":"nested only"}]}"#.as_slice(),
        br#"{"model":null}"#.as_slice(),
        br#"{"model":42}"#.as_slice(),
        br#"{"model":{"nested":[true,"text"]}}"#.as_slice(),
        br#"[]"#.as_slice(),
    ] {
        let mut outputs = Vec::new();
        // The conditional template forces the existing full-JSON path while
        // rendering the same value, providing a compatibility reference.
        for template in ["{{ model }}", "{% if true %}{{ model }}{% endif %}"] {
            for buffered in [false, true] {
                let config_json = serde_json::json!({
                    "request": {
                        "body": {"parseAs": "AsJson"},
                        "set": [
                            {"name": "Modal-Inference-Endpoint", "value": template},
                            {"name": "Modal-Inference-Route", "value": "1"}
                        ]
                    }
                })
                .to_string();
                let config = FilterConfig::new(&config_json).unwrap();
                assert_eq!(
                    config.request_json_fields.is_some(),
                    template == "{{ model }}"
                );
                let mut envoy = MockEnvoyHttpFilter::default();
                // Exercise the per-route config path used by the chat route.
                envoy
                    .expect_get_most_specific_route_config()
                    .returning(move || Some(Arc::new(config.clone())));
                envoy.expect_get_request_headers().returning(Vec::new);
                let bytes = body.to_vec();
                let chunks = move || {
                    let bytes = Box::leak(bytes.clone().into_boxed_slice());
                    let (first, rest) = bytes.split_at_mut(1);
                    // These disjoint buffers remain valid for the filter's lifetime.
                    Some(unsafe { vec![EnvoyMutBuffer::new(first), EnvoyMutBuffer::new(rest)] })
                };
                if buffered {
                    envoy.expect_get_buffered_request_body().returning(chunks);
                    envoy.expect_get_received_request_body().times(0);
                } else {
                    envoy.expect_get_buffered_request_body().returning(|| None);
                    envoy.expect_get_received_request_body().returning(chunks);
                }
                let headers = Arc::new(Mutex::new(Vec::new()));
                let set_headers = headers.clone();
                envoy
                    .expect_set_request_header()
                    .returning(move |key, value| {
                        set_headers
                            .lock()
                            .unwrap()
                            .push((key.to_owned(), Some(value.to_vec())));
                        true
                    });
                let removed_headers = headers.clone();
                envoy.expect_remove_request_header().returning(move |key| {
                    removed_headers.lock().unwrap().push((key.to_owned(), None));
                    true
                });
                envoy.expect_drain_buffered_request_body().times(0);
                envoy.expect_drain_received_request_body().times(0);
                envoy.expect_append_buffered_request_body().times(0);
                envoy.expect_append_received_request_body().times(0);

                let base = FilterConfig::new("{}").unwrap();
                let mut filter = base.new_http_filter(&mut envoy);
                assert_eq!(filter.on_request_headers(&mut envoy, false),
                    abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::StopIteration);
                assert_eq!(filter.on_request_body(&mut envoy, false),
                    abi::envoy_dynamic_module_type_on_http_filter_request_body_status::StopIterationAndBuffer);
                assert_eq!(
                    filter.on_request_body(&mut envoy, true),
                    abi::envoy_dynamic_module_type_on_http_filter_request_body_status::Continue
                );
                outputs.push(headers.lock().unwrap().clone());
            }
        }
        assert!(
            outputs.windows(2).all(|pair| pair[0] == pair[1]),
            "{body:?}: {outputs:?}"
        );
        assert!(outputs[0].contains(&("Modal-Inference-Route".to_owned(), Some(b"1".to_vec()))));
    }
}
