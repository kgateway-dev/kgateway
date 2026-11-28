#![allow(clippy::unwrap_used, clippy::expect_used)]

use super::*;
use mockall::*;

#[test]
fn local_reply_can_reenter_response_headers() {
    use std::rc::Rc;

    let mut envoy_filter = MockEnvoyHttpFilter::default();
    let config = FilterConfig::new(
        r#"{
            "request": { "body": { "parseAs": "AsJson" } },
            "response": {
                "set": [{ "name": "X-Original", "value": "{{ request_header(\"x-original\") }}" }]
            }
        }"#,
    )
    .expect("valid filter config");
    let filter: Rc<dyn HttpFilter<MockEnvoyHttpFilter>> =
        Rc::from(config.new_http_filter(&mut envoy_filter));
    envoy_filter
        .expect_get_most_specific_route_config()
        .returning(|| None);
    envoy_filter.expect_get_request_headers().returning(|| {
        vec![(
            EnvoyBuffer::new(b"x-original"),
            EnvoyBuffer::new(b"original"),
        )]
    });
    envoy_filter
        .expect_get_buffered_request_body()
        .returning(|| None);
    envoy_filter
        .expect_get_received_request_body()
        .returning(|| {
            let body = Box::leak(b"invalid json".to_vec().into_boxed_slice());
            // SAFETY: each call owns a fresh allocation that outlives the buffer.
            Some(vec![unsafe { EnvoyMutBuffer::new(body) }])
        });
    let reentrant_filter = Rc::clone(&filter);
    envoy_filter
        .expect_send_response()
        .times(1)
        .returning_st(move |status, _, _, _| {
            assert_eq!(status, 400);
            let mut response_filter = MockEnvoyHttpFilter::default();
            response_filter
                .expect_get_most_specific_route_config()
                .returning(|| None);
            // A nested callback must retain the original request header snapshot.
            response_filter
                .expect_get_request_headers()
                .returning(Vec::new);
            response_filter
                .expect_get_response_headers()
                .returning(Vec::new);
            response_filter
                .expect_set_response_header()
                .withf(|key, value| key == "X-Original" && value == b"original")
                .times(1)
                .return_const(true);
            assert_eq!(
                reentrant_filter.on_response_headers(&mut response_filter, true),
                abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::Continue
            );
        });

    assert_eq!(
        filter.on_request_body(&mut envoy_filter, true),
        abi::envoy_dynamic_module_type_on_http_filter_request_body_status::StopIterationAndBuffer
    );
}

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
    let filter = filter_conf.new_http_filter(&mut envoy_filter);

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
    let filter = filter_conf.new_http_filter(&mut envoy_filter);

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
    let filter = filter_conf.new_http_filter(&mut envoy_filter);

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
    let filter = filter_conf.new_http_filter(&mut envoy_filter);

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

/// Body storage shared by the SDK mock's read and mutation callbacks.
#[derive(Clone)]
struct BodyBuffer(std::rc::Rc<std::cell::RefCell<Vec<u8>>>);

impl BodyBuffer {
    fn new(bytes: &[u8]) -> Self {
        Self(std::rc::Rc::new(std::cell::RefCell::new(bytes.to_vec())))
    }

    fn chunks(&self) -> Option<Vec<EnvoyMutBuffer<'static>>> {
        let mut bytes = self.0.borrow_mut();
        if bytes.is_empty() {
            return None;
        }
        // SAFETY: the test owns the allocation throughout the filter callback. The
        // mock only mutates it after the filter releases its borrowed SDK buffers.
        Some(vec![unsafe { EnvoyMutBuffer::new(bytes.as_mut_slice()) }])
    }

    fn drain(&self, count: usize) -> bool {
        let mut bytes = self.0.borrow_mut();
        let count = count.min(bytes.len());
        bytes.drain(..count);
        true
    }

    fn append(&self, bytes: &[u8]) -> bool {
        self.0.borrow_mut().extend_from_slice(bytes);
        true
    }

    fn len(&self) -> usize {
        self.0.borrow().len()
    }
}

#[test]
fn request_transform_includes_final_data_frame() {
    use abi::{
        envoy_dynamic_module_type_on_http_filter_request_body_status as BodyStatus,
        envoy_dynamic_module_type_on_http_filter_request_headers_status as HeadersStatus,
    };

    let config = FilterConfig::new(r#"{"request":{"body":{"value":"[{{ body() }}]"}}}"#).unwrap();
    for (prefix, tail, aliased) in [
        ("AAAA", "ZZZZ", false),
        ("AAAAZZZZ", "", false),
        ("", "AAAAZZZZ", false),
        ("AAAAZZZZ", "", true),
    ] {
        let mut envoy = MockEnvoyHttpFilter::default();
        envoy
            .expect_get_most_specific_route_config()
            .returning(|| None);
        envoy.expect_get_request_headers().returning(Vec::new);
        let filter = config.new_http_filter(&mut envoy);
        assert_eq!(
            filter.on_request_headers(&mut envoy, false),
            HeadersStatus::StopIteration
        );

        if !prefix.is_empty() && !aliased {
            assert_eq!(
                filter.on_request_body(&mut envoy, false),
                BodyStatus::StopIterationAndBuffer
            );
        }

        // Envoy has moved the earlier frame into its buffer. The final callback
        // sees the current frame separately, unless Envoy is replaying its buffer.
        let buffered = BodyBuffer::new(prefix.as_bytes());
        let received = if aliased {
            buffered.clone()
        } else {
            BodyBuffer::new(tail.as_bytes())
        };
        envoy.expect_get_buffered_request_body().returning_st({
            let buffered = buffered.clone();
            move || buffered.chunks()
        });
        envoy.expect_get_received_request_body().returning_st({
            let received = received.clone();
            move || received.chunks()
        });
        envoy.expect_get_buffered_request_body_size().returning_st({
            let buffered = buffered.clone();
            move || buffered.len()
        });
        envoy
            .expect_received_buffered_request_body()
            .return_const(aliased);
        envoy.expect_drain_buffered_request_body().returning_st({
            let buffered = buffered.clone();
            move |count| buffered.drain(count)
        });
        envoy.expect_drain_received_request_body().returning_st({
            let received = received.clone();
            move |count| received.drain(count)
        });
        envoy.expect_append_buffered_request_body().returning_st({
            let buffered = buffered.clone();
            move |bytes| buffered.append(bytes)
        });
        envoy.expect_append_received_request_body().returning_st({
            let received = received.clone();
            move |bytes| received.append(bytes)
        });
        let captured_length = std::rc::Rc::new(Cell::new(0));
        envoy.expect_set_request_header().times(1).returning_st({
            let captured_length = captured_length.clone();
            move |key, value| {
                assert_eq!(key, "content-length");
                captured_length.set(std::str::from_utf8(value).unwrap().parse().unwrap());
                true
            }
        });

        assert_eq!(
            filter.on_request_body(&mut envoy, true),
            BodyStatus::Continue
        );
        let mut forwarded = buffered.0.borrow().clone();
        if !aliased {
            forwarded.extend_from_slice(&received.0.borrow());
        }
        assert_eq!(
            std::str::from_utf8(&forwarded).unwrap(),
            "[AAAAZZZZ]",
            "prefix={prefix:?}, final frame={tail:?}, aliased={aliased}"
        );
        assert_eq!(captured_length.get(), forwarded.len());
    }
}

#[test]
fn response_transform_includes_final_data_frame_in_json() {
    use abi::{
        envoy_dynamic_module_type_on_http_filter_response_body_status as BodyStatus,
        envoy_dynamic_module_type_on_http_filter_response_headers_status as HeadersStatus,
    };

    let config = FilterConfig::new(
        r#"{"response":{"body":{"parseAs":"AsJson","value":"{{ model }}:{{ body() }}"}}}"#,
    )
    .unwrap();
    let mut envoy = MockEnvoyHttpFilter::default();
    envoy
        .expect_get_most_specific_route_config()
        .returning(|| None);
    envoy.expect_get_request_headers().returning(Vec::new);
    envoy.expect_get_response_headers().returning(Vec::new);
    let filter = config.new_http_filter(&mut envoy);
    assert_eq!(
        filter.on_response_headers(&mut envoy, false),
        HeadersStatus::StopIteration
    );
    assert_eq!(
        filter.on_response_body(&mut envoy, false),
        BodyStatus::StopIterationAndBuffer
    );

    let buffered = BodyBuffer::new(br#"{"model":"gpt-"#);
    let received = BodyBuffer::new(br#"4"}"#);
    envoy.expect_get_buffered_response_body().returning_st({
        let buffered = buffered.clone();
        move || buffered.chunks()
    });
    envoy.expect_get_received_response_body().returning_st({
        let received = received.clone();
        move || received.chunks()
    });
    envoy
        .expect_get_buffered_response_body_size()
        .returning_st({
            let buffered = buffered.clone();
            move || buffered.len()
        });
    envoy
        .expect_received_buffered_response_body()
        .return_const(false);
    envoy.expect_drain_buffered_response_body().returning_st({
        let buffered = buffered.clone();
        move |count| buffered.drain(count)
    });
    envoy.expect_drain_received_response_body().returning_st({
        let received = received.clone();
        move |count| received.drain(count)
    });
    envoy.expect_append_buffered_response_body().returning_st({
        let buffered = buffered.clone();
        move |bytes| buffered.append(bytes)
    });
    envoy.expect_set_response_header().returning(|key, value| {
        assert_eq!(key, "content-length");
        assert_eq!(value, b"23");
        true
    });
    envoy.expect_send_response().returning(|status, _, _, _| {
        panic!("complete response JSON should render, got HTTP {status}");
    });

    assert_eq!(
        filter.on_response_body(&mut envoy, true),
        BodyStatus::Continue
    );
    assert_eq!(
        buffered.0.borrow().as_slice(),
        br#"gpt-4:{"model":"gpt-4"}"#
    );
    assert_eq!(received.len(), 0);
}
