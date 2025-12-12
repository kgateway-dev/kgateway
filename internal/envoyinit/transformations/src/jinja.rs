use crate::LocalTransform;
use crate::NameValuePair;
use crate::TransformationOps;
use anyhow::{Context, Error, Result};
use base64::{
    engine::general_purpose::{STANDARD, STANDARD_NO_PAD},
    Engine,
};
use minijinja::{context, Environment, State};
use rand::Rng;
use serde::Deserialize;
use std::collections::HashMap;
use std::env;

// substring can be called with either two or three arguments --
// the first argument is the string to be modified, the second is the start position
// of the substring, and the optional third argument is the length of the substring.
// If the third argument is not provided or invalid, the substring will extend to
// the end of the string.
fn substring(input: &str, start: usize, len: Option<usize>) -> String {
    let input_len = input.len();
    if start >= input_len {
        return "".to_string();
    }

    let mut end = input_len;
    if let Some(len) = len {
        if start + len <= input_len {
            end = start + len
        }
    }

    input[start..end].to_string()
}

fn header(state: &State, key: &str) -> String {
    let headers = state.lookup("headers");
    let Some(headers) = headers else {
        return "".to_string();
    };

    let Some(header_map) = <HashMap<String, String>>::deserialize(headers.clone()).ok() else {
        return "".to_string();
    };

    header_map.get(key).cloned().unwrap_or_default()
}

fn request_header(state: &State, key: &str) -> String {
    let headers = state.lookup("request_headers");
    let Some(headers) = headers else {
        return "".to_string();
    };

    let Some(header_map) = <HashMap<String, String>>::deserialize(headers.clone()).ok() else {
        return "".to_string();
    };
    header_map.get(key).cloned().unwrap_or_default()
}

fn base64_encode(input: &[u8]) -> String {
    STANDARD.encode(input)
}

fn base64_decode(input: &str) -> String {
    STANDARD
        .decode(input)
        .ok()
        .and_then(|bytes| String::from_utf8(bytes).ok())
        .unwrap_or_default()
}

fn get_env(env_var: &str) -> String {
    match env::var(env_var) {
        Ok(val) => val,
        Err(_e) => "".to_string(),
    }
}

fn replace_with_random(input: &str, to_replace: &str) -> String {
    // TODO: in the C++ version, the pattern is generated once per "to_replace" string
    //       and get re-used for all calls within the request context but I cannot find
    //       a way to do this here yet
    let mut rng = rand::rng();
    let high: u64 = rng.random();
    let low: u64 = rng.random();
    let mut random = [0u8; 16];
    random[..8].copy_from_slice(&low.to_le_bytes());
    random[8..].copy_from_slice(&high.to_le_bytes());

    let pattern = STANDARD_NO_PAD.encode(random);
    input.replace(to_replace, &pattern)
}

pub fn new_jinja_env() -> Environment<'static> {
    let mut env = Environment::new();

    env.add_function("env", get_env);
    env.add_function("substring", substring);

    // !! Standard string manipulation
    // env.add_function("trim", trim);
    env.add_function("base64_encode", base64_encode);
    // env.add_function("base64url_encode", base64url_encode);
    env.add_function("base64_decode", base64_decode);
    // env.add_function("base64url_decode", base64url_decode);
    env.add_function("replace_with_random", replace_with_random);
    // env.add_function("raw_string", raw_string);
    //        env.add_function("word_count", word_count);

    // !! Envoy context accessors
    env.add_function("header", header);
    env.add_function("request_header", request_header);
    // env.add_function("extraction", extraction);
    // env.add_function("body", body);
    // env.add_function("dynamic_metadata", dynamic_metadata);

    // !! Datasource Puller needed
    // env.add_function("data_source", data_source);

    // !! Requires being in an upstream filter
    // env.add_function("host_metadata", host_metadata);
    // env.add_function("cluster_metadata", cluster_metadata);

    // !! Possibly not relevant old inja internal debug stuff
    // env.add_function("context", context);
    // env.add_function("env", env);

    // specific.extend(self.route_specific.into_iter());

    env
}

fn render(env: &Environment<'static>, ctx: minijinja::Value, template: &str) -> Result<String> {
    let tmpl = env
        .template_from_str(template)
        .context("error creating jinja template {template}")?;
    tmpl.render(ctx)
        .context("error rendering jinja template {template}")
}

fn combine_errors(msg: &str, errors: Vec<Error>) -> Result<()> {
    if !errors.is_empty() {
        let combined = errors
            .into_iter()
            .map(|e| e.to_string())
            .collect::<Vec<_>>()
            .join("; ");
        return Err(anyhow::anyhow!("{}: {}", msg, combined));
    }

    Ok(())
}

/// Transform Request Headers
///
/// On any rendering errors, we will remove the header and continue
/// All the errors are collected and bubble up the chain so they can be logged
pub fn transform_request_headers<T: TransformationOps>(
    transform: &LocalTransform,
    env: &Environment<'static>,
    request_headers_map: &HashMap<String, String>,
    mut ops: T,
) -> Result<()> {
    let mut errors = Vec::new();

    for NameValuePair { name: key, value } in &transform.set {
        if value.is_empty() {
            // This is following the legacy transformation filter behavior
            ops.remove_request_header(key);
            continue;
        }
        let rendered = match render(
            env,
            // for request rendering, both the header() and request_header() use the request_headers
            // so, setting both to the request_headers_map in the context
            context!(headers => request_headers_map, request_headers => request_headers_map),
            value,
        ) {
            Ok(str) => Some(str),
            Err(e) => {
                errors.push(e);
                None
            }
        };

        if rendered.as_deref().is_some_and(|s| !s.is_empty()) {
            ops.set_request_header(key, rendered.as_deref().unwrap().as_bytes());
        } else {
            ops.remove_request_header(key);
        }
    }

    // TODO: "add" header is not supported by the rust SDK yet

    for key in &transform.remove {
        ops.remove_request_header(key);
    }

    combine_errors("transform_request_headers()", errors)
}

/// Transform Resposne Headers
///
/// On any rendering errors, we will remove the header and continue
/// All the errors are collected and bubble up the chain so they can be logged
pub fn transform_response_headers<T: TransformationOps>(
    transform: &LocalTransform,
    env: &Environment<'static>,
    request_headers_map: &HashMap<String, String>,
    response_headers_map: &HashMap<String, String>,
    mut ops: T,
) -> Result<()> {
    let mut errors = Vec::new();

    for NameValuePair { name: key, value } in &transform.set {
        if value.is_empty() {
            // This is following the legacy transformation filter behavior
            ops.remove_response_header(key);
            continue;
        }
        let rendered = match render(
            env,
            // for response rendering, header() uses response_headers and request_header()
            // uses the request_headers. So, setting them in the context accordingly
            context!(headers => response_headers_map, request_headers => request_headers_map),
            value,
        ) {
            Ok(str) => Some(str),
            Err(e) => {
                errors.push(e);
                None
            }
        };

        if rendered.as_deref().is_some_and(|s| !s.is_empty()) {
            ops.set_response_header(key, rendered.as_deref().unwrap().as_bytes());
        } else {
            ops.remove_response_header(key);
        }
    }

    // TODO: "add" header is not supported by the rust SDK yet

    for key in &transform.remove {
        ops.remove_response_header(key);
    }

    combine_errors("transform_response_headers()", errors)
}
