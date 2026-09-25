#![allow(clippy::unwrap_used, clippy::expect_used)]

use super::*;
use serde_json::json;

fn transform(template: &str) -> LocalTransform {
    serde_json::from_value(json!({
        "body": {"parseAs": "AsJson"},
        "set": [
            {"name": "Modal-Inference-Endpoint", "value": template},
            {"name": "Modal-Inference-Route", "value": "1"}
        ]
    }))
    .unwrap()
}

#[test]
fn selects_only_simple_header_lookups() {
    for template in ["{{ model }}", "{{model}}", "{{\nmodel\t}}"] {
        assert_eq!(
            requested_fields(&transform(template)),
            Some(vec!["model".into()])
        );
    }
    for template in [
        "1",
        "{{ }}",
        "{{ model.name }}",
        "{{ model | string }}",
        "{{ context() }}",
        "{{ header('x-model') }}",
        "prefix {{ model }}",
        "{{ model }} {{ other }}",
        "{% if model %}yes{% endif %}",
        "{# comment #}{{ model }}",
        "{{- model -}}",
    ] {
        assert_eq!(requested_fields(&transform(template)), None, "{template}");
    }
    let mut config = transform("{{ model }}");
    config.add = vec![
        transformation::NameValuePair {
            name: "X-Model".into(),
            value: "{{model}}".into(),
        },
        transformation::NameValuePair {
            name: "X-Stream".into(),
            value: "{{ stream }}".into(),
        },
    ];
    assert_eq!(
        requested_fields(&config),
        Some(vec!["model".into(), "stream".into()])
    );
    config.body.as_mut().unwrap().value = "{{ context() }}".into();
    assert_eq!(requested_fields(&config), None);
    config.body.as_mut().unwrap().value.clear();
    config.body.as_mut().unwrap().parse_as = BodyParseBehavior::AsString;
    assert_eq!(requested_fields(&config), None);
    config.body.as_mut().unwrap().parse_as = BodyParseBehavior::AsJson;
    config.dynamic_metadata = serde_json::from_value(json!([
        {"namespace": "test", "key": "body", "value": {"stringValue": "{{ model }}"}}
    ]))
    .unwrap();
    assert_eq!(requested_fields(&config), None);
}

#[test]
fn matches_full_parser_for_selected_fields_and_errors() {
    let fields = vec!["model".to_owned(), "stream".to_owned()];
    let mut inputs = vec![
        br#"{"model":"first","messages":[{"model":"nested","content":"hello"}]}"#.to_vec(),
        br#"{"messages":[{"content":"hello"}],"model":"last","stream":true}"#.to_vec(),
        br#"{"model":"first","mo\u0064el":"escaped last","other":null}"#.to_vec(),
        br#"{"model":{"a":[1,true,null]},"unused":[false,-1,2.5,18446744073709551615]}"#.to_vec(),
        br#"{"model":null,"stream":false,"unused":"\uD83D\uDE00"}"#.to_vec(),
        br#"{"model":7,"messages":[],"other":{"key":1,"key":2}}"#.to_vec(),
        br#"{"messages":[{"model":"nested only"}]}"#.to_vec(),
        b"{}".to_vec(),
        b"null".to_vec(),
        b"true".to_vec(),
        b"42".to_vec(),
        br#""string root""#.to_vec(),
        b"[1,2,{\"model\":\"nested\"}]".to_vec(),
        b"".to_vec(),
        b" \n\t".to_vec(),
        br#"{"model":"ok","ignored":[1,]}"#.to_vec(),
        br#"{"model":"ok","ignored":{"nested":1e400}}"#.to_vec(),
        br#"{"model":"ok","ignored":"\uD800"}"#.to_vec(),
        br#"{"model":"ok","ignored":{"\uD800":1}}"#.to_vec(),
        br#"{"model":"ok","ignored":"\x01"}"#.to_vec(),
        br#"{"model":"ok"} {"model":"trailing"}"#.to_vec(),
        br#"{"model":"ok","model":}"#.to_vec(),
        b"{\"model\":\"ok\",\"ignored\":\"\xff\"}".to_vec(),
        format!("{}{{\"model\":\"padded\"}}", " ".repeat(10_000)).into_bytes(),
    ];
    for depth in [124, 125, 126, 127, 128, 129, 200] {
        inputs.push(
            format!(
                "{{\"model\":\"ok\",\"ignored\":{}0{}}}",
                "[".repeat(depth),
                "]".repeat(depth)
            )
            .into_bytes(),
        );
    }
    for input in inputs {
        let full = serde_json::from_reader::<_, Value>(input.as_slice()).map(|mut value| {
            if let Value::Object(ref mut object) = value {
                object.retain(|key, _| fields.contains(key));
            }
            value
        });
        let selected = parse(input.as_slice(), &fields);
        match (full, selected) {
            (Ok(expected), Ok(actual)) => assert_eq!(actual, expected, "{input:?}"),
            (Err(expected), Err(actual)) => {
                let actual = actual.downcast_ref::<serde_json::Error>().unwrap();
                assert_eq!(actual.classify(), expected.classify(), "{input:?}");
            }
            (expected, actual) => {
                panic!("parser mismatch for {input:?}: {expected:?} vs {actual:?}")
            }
        }
    }
}

#[test]
fn discards_large_prompt_and_reads_model_at_end() {
    let content = "some prompt text ".repeat(30_000);
    let body =
        serde_json::to_vec(&json!({"messages": [{"content": content}], "model": "test-model"}))
            .unwrap();
    let fields = vec!["model".to_owned()];
    let reader = std::io::Cursor::new(&body[..7]).chain(std::io::Cursor::new(&body[7..]));
    assert_eq!(
        parse(reader, &fields).unwrap(),
        json!({"model": "test-model"})
    );
}

#[test]
#[ignore = "microbenchmark: run with --release --ignored --nocapture"]
fn benchmark_model_extraction() {
    use std::hint::black_box;
    use std::time::Instant;

    let body = serde_json::to_vec(&json!({
        "model": "test-model",
        "messages": (0..100).map(|_| json!({"role": "user", "content": "hello world ".repeat(400)})).collect::<Vec<_>>()
    })).unwrap();
    let fields = vec!["model".to_owned()];
    let iterations = 200;
    for selective in [false, true] {
        let start = Instant::now();
        for _ in 0..iterations {
            let value = if selective {
                parse(black_box(body.as_slice()), &fields).unwrap()
            } else {
                serde_json::from_reader(black_box(body.as_slice())).unwrap()
            };
            let Value::Object(object) = value else {
                panic!("expected object")
            };
            let context: std::collections::HashMap<_, _> = object
                .into_iter()
                .map(|(key, value)| (key, minijinja::Value::from_serialize(&value)))
                .collect();
            black_box(context);
        }
        eprintln!(
            "selective={selective}, bytes={}, iterations={iterations}, elapsed={:?}",
            body.len(),
            start.elapsed()
        );
    }
}
