#![deny(clippy::unwrap_used, clippy::expect_used)]

use crate::BodyParseBehavior;
use crate::LocalTransform;
use crate::LocalTransformationConfig;
use crate::NameValuePair;
use crate::TransformationError;
use crate::TransformationOps;
use anyhow::{Context, Error, Result};
use bitflags::bitflags;

bitflags! {
    #[derive(Clone, Copy, Debug, PartialEq, Eq)]
    pub struct ProcessFlags: u8 {
        const HEADER = 0b01;
        const BODY   = 0b10;
    }
}
use base64::{
    engine::general_purpose::{STANDARD, STANDARD_NO_PAD, URL_SAFE},
    Engine,
};
use minijinja::{Environment, State};
use once_cell::sync::Lazy;
use rand::Rng;
use serde::Deserialize;
use serde_json::Value as JsonValue;
use std::collections::{HashMap, HashSet};
use std::env;

// These keys are used in a shared scope in the State where we will also put the parsed json body in.
// So, they needs to be as uniq as possible to minimize collision.
const STATE_LOOKUP_KEY_BODY: &str = "body.dev.kgateway";
const STATE_LOOKUP_KEY_CONTEXT: &str = "context.dev.kgateway";
const STATE_LOOKUP_KEY_HEADERS: &str = "headers.dev.kgateway";
const STATE_LOOKUP_KEY_REQ_HEADERS: &str = "request_headers.dev.kgateway";

const REQUEST_BODY_TEMPLATE_LOOKUP_KEY: &str = "request_body_0";
const RESPONSE_BODY_TEMPLATE_LOOKUP_KEY: &str = "response_body_0";

static ENV: Lazy<Environment<'static>> = Lazy::new(new_jinja_env);

static GLOBALS_LOOKUP: Lazy<HashSet<&'static str>> =
    Lazy::new(|| ENV.globals().map(|(k, _)| k).collect());

// substring can be called with either two or three arguments --
// the first argument is the string to be modified, the second is the start position
// of the substring, and the optional third argument is the length of the substring.
// If the third argument is not provided or invalid, the substring will extend to
// the end of the string.
fn substring(input: &str, start: usize, len: Option<usize>) -> String {
    let input_len = input.len();
    if start >= input_len {
        return String::default();
    }

    let mut end = input_len;
    if let Some(len) = len {
        if start + len <= input_len {
            end = start + len
        }
    }

    input[start..end].to_string()
}

fn lookup_header(headers: Option<minijinja::Value>, key: &str) -> String {
    let Some(headers) = headers else {
        return String::default();
    };

    // TODO: can this be cached at a per request/response context somehow?
    //       This is called inside a custom function registered to minijina and
    //       we only get the State object which can only contain minijina::Value
    //       when we get called.
    let Some(header_map) = <HashMap<String, String>>::deserialize(headers.clone()).ok() else {
        return String::default();
    };
    let lowercase_key = key.to_lowercase();
    header_map.get(&lowercase_key).cloned().unwrap_or_default()
}

fn header(state: &State, key: &str) -> String {
    let headers = state.lookup(STATE_LOOKUP_KEY_HEADERS);
    lookup_header(headers, key)
}

fn request_header(state: &State, key: &str) -> String {
    let headers = state.lookup(STATE_LOOKUP_KEY_REQ_HEADERS);
    lookup_header(headers, key)
}

fn trim_outer_quotes(s: &str) -> &str {
    if s.starts_with('"') && s.ends_with('"') && s.len() >= 2 {
        &s[1..s.len() - 1]
    } else {
        s
    }
}

fn raw_string(value: &str) -> String {
    // Not sure if this is exactly the correct behavior for this function. In the C++ version,
    // the native json object can be added to the context directly and that json object can dump
    // out the raw string without un-escaping. Here, it's several layers of deserializing and serializing
    // from serde_json::from_slice() -> constructing a BTreeMap -> adding that to the context.
    // There is no way to get back the original raw_string. So, escaping the string again is the closest I
    // can get. After escaping, the resulting string has extra double quote around the original string, so
    // need to trim them
    // Interesting Note: somehow the need for trimming the double quotes is exactly the same in the C++
    // code. So, maybe the C++ json object dumps() is also doing something similar behind the scene
    match serde_json::to_string(value) {
        Ok(s) => trim_outer_quotes(&s).to_string(),
        Err(_) => String::default(),
    }
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

fn base64url_encode(input: &[u8]) -> String {
    URL_SAFE.encode(input)
}

fn base64url_decode(input: &str) -> String {
    URL_SAFE
        .decode(input)
        .ok()
        .and_then(|bytes| String::from_utf8(bytes).ok())
        .unwrap_or_default()
}

fn get_env(env_var: &str) -> String {
    env::var(env_var).unwrap_or_default()
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

fn replace_with_string(input: &str, to_replace: &str, with_string: &str) -> String {
    input.replace(to_replace, with_string)
}

fn body(state: &State) -> String {
    state
        .lookup(STATE_LOOKUP_KEY_BODY)
        .unwrap_or_default()
        .to_string()
}

fn context(state: &State) -> minijinja::Value {
    state.lookup(STATE_LOOKUP_KEY_CONTEXT).unwrap_or_default()
}

pub fn new_jinja_env() -> Environment<'static> {
    let mut env = Environment::new();

    // if parseAsJson is used for body parsing. minijinja would prefer the json instead of custom function
    // when rendering the template. For example, we have this `env()` function here, if the json body also has
    // a field named `env`, the `env()` call in the template will fail to be rendered because minijinja resolves
    // `env` to the json value from the body and then will complain it's not callable.
    // If we are adding any new functions, we should make the function name more uniq to minimize the chance
    // of collision.
    env.add_function("env", get_env);
    env.add_function("substring", substring);

    // !! Standard string manipulation
    // env.add_function("trim", trim);
    env.add_function("base64_encode", base64_encode);
    env.add_function("base64url_encode", base64url_encode);
    env.add_function("base64_decode", base64_decode);
    env.add_function("base64url_decode", base64url_decode);
    env.add_function("replace_with_random", replace_with_random);
    env.add_function("replace_with_string", replace_with_string);
    env.add_function("raw_string", raw_string);
    //        env.add_function("word_count", word_count);

    // !! Envoy context accessors
    env.add_function("header", header);
    env.add_function("request_header", request_header);
    // env.add_function("extraction", extraction);
    env.add_function("body", body);
    // env.add_function("dynamic_metadata", dynamic_metadata);

    // !! Datasource Puller needed
    // env.add_function("data_source", data_source);

    // !! Requires being in an upstream filter
    // env.add_function("host_metadata", host_metadata);
    // env.add_function("cluster_metadata", cluster_metadata);

    // !! Possibly not relevant old inja internal debug stuff
    env.add_function("context", context);

    env
}

// For headers, the template lookup key is the same as the template strings.
// For bodies, because there will only be 1 for request and 1 for response, we use
// a short key when we compile the templates. So, pass in RESPONSE_BODY_TEMPLATE_LOOKUP_KEY
// or REQUEST_BODY_TEMPLATE_LOOKUP_KEY for template_key when rendering body.
// pass in the same template string as the template_key for headers.
fn render(
    env: &Environment<'static>,
    ctx: &minijinja::Value,
    template_key: &str,
    template: &str,
    parsed_body_as_json: bool,
) -> Result<String> {
    if template.is_empty() {
        return Ok(String::new());
    }
    let tmpl = env
        .get_template(template_key)
        .with_context(|| format!("error looking up jinja template {}", template))?;
    if !parsed_body_as_json {
        // This is to mimic the C++ behavior when a transformation is used that needs
        // the body parsed as json but it's not enabled. So, we try to detect if
        // the transformation template has any undeclared variables when parseAsJson
        // is not turned on. Returning a TransformationError type here will cause
        // the envoy layer code to return a local reply with 400 status code.
        // Other errors would be logged but they are not critical to stop the request
        let undeclared_variables = tmpl.undeclared_variables(true);
        if !undeclared_variables.is_empty() {
            for v in &undeclared_variables {
                // Unfortunately, custom function is also reported as undeclared variables
                // by minijinja, so only return error if the undeclared variables are not
                // custom functions. GLOBALS_LOCKUP is lazily constructed once and is
                // static throughout the lifetime of the process.
                if !GLOBALS_LOOKUP.contains(v.as_str()) {
                    return Err(TransformationError::UndeclaredJsonVariables(format!(
                        "{:?} from template {}",
                        undeclared_variables, template
                    ))
                    .into());
                }
            }
        }
    }
    tmpl.render(ctx)
        .with_context(|| format!("error rendering jinja template {}", template))
}

fn combine_errors(msg: &str, errors: Vec<Error>) -> Result<()> {
    // Each error can have multiple level of errors, that's why there is
    // the e.chain() iterating through each error and combine them
    if !errors.is_empty() {
        let combined = errors
            .into_iter()
            .map(|e| {
                e.chain()
                    .map(|cause| cause.to_string())
                    .collect::<Vec<String>>()
                    .join(":")
            })
            .collect::<Vec<_>>()
            .join("; ");
        return Err(anyhow::anyhow!("{}: {}", msg, combined));
    }

    Ok(())
}

/// Renders a template and checks for abort conditions (UndeclaredJsonVariables).
/// Returns Err if we must abort, Ok(None) if render failed but error was collected,
/// Ok(Some(s)) on success.
fn render_or_abort(
    env: &Environment<'static>,
    ctx: &minijinja::Value,
    template_key: &str,
    template: &str,
    parsed_body_as_json: bool,
    errors: &mut Vec<Error>,
) -> Result<Option<String>> {
    match render(env, ctx, template_key, template, parsed_body_as_json) {
        Ok(s) => Ok(Some(s)),
        Err(err) => {
            if err
                .downcast_ref::<TransformationError>()
                .is_some_and(|e| matches!(e, TransformationError::UndeclaredJsonVariables(_)))
            {
                return Err(err);
            }
            errors.push(err);
            Ok(None)
        }
    }
}

/// Inserts a parsed JSON body into the context map.
/// Returns true if the body was non-null and inserted, false otherwise.
fn add_context_from_json_body(
    m: &mut HashMap<String, minijinja::Value>,
    json_body: JsonValue,
    body_template_value: &str,
) -> bool {
    if json_body == JsonValue::Null {
        return false;
    }
    if body_template_value.contains("context()") {
        m.insert(
            STATE_LOOKUP_KEY_CONTEXT.to_string(),
            minijinja::Value::from_serialize(&json_body),
        );
    }
    if let JsonValue::Object(map) = json_body {
        for (k, v) in map {
            m.insert(k, minijinja::Value::from_serialize(&v));
        }
    }
    true
}

struct BodyOps<T> {
    drain_body: fn(&mut T, usize) -> bool,
    set_header: fn(&mut T, &str, &[u8]) -> bool,
    append_body: fn(&mut T, &[u8]) -> bool,
    remove_header: fn(&mut T, &str) -> bool,
}

/// Renders the body template, drains the old body, and sets the new body + content-length.
/// If rendering fails or produces an empty result, sets content-length to 0 and removes
/// content-type.
fn render_and_set_body<T: TransformationOps>(
    env: &Environment<'static>,
    ctx: &minijinja::Value,
    body_value: &str,
    template_key: &str,
    parsed_body_as_json: bool,
    ops: &mut T,
    body_ops: BodyOps<T>,
) -> Vec<Error> {
    let BodyOps {
        drain_body,
        set_header,
        append_body,
        remove_header,
    } = body_ops;
    let mut errors = Vec::new();
    drain_body(ops, usize::MAX);
    let rendered = match render(env, ctx, template_key, body_value, parsed_body_as_json) {
        Ok(str) => Some(str),
        Err(e) => {
            errors.push(e);
            None
        }
    };
    if let Some(rendered_str) = rendered.as_deref().filter(|s| !s.is_empty()) {
        let rendered_body = rendered_str.as_bytes();
        set_header(
            ops,
            "content-length",
            rendered_body.len().to_string().as_bytes(),
        );
        append_body(ops, rendered_body);
    } else {
        set_header(ops, "content-length", b"0");
        // In classic transformation, we remove content-type only when "passthrough_body"
        // is set to true (even the body is not transformed but it comes in as 0 bytes)
        // Here, we are only removing content-type if we have an override that ended up
        // removing the body as we don't have passthrough_body setting in kgateway
        remove_header(ops, "content-type");
    }
    errors
}

struct HeaderOps<T> {
    set: fn(&mut T, &str, &[u8]) -> bool,
    add: fn(&mut T, &str, &[u8]) -> bool,
    remove: fn(&mut T, &str) -> bool,
}

fn process_headers<T: TransformationOps>(
    env: &Environment<'static>,
    ctx: &minijinja::Value,
    transform: &LocalTransform,
    parsed_body_as_json: bool,
    errors: &mut Vec<Error>,
    ops: &mut T,
    header_ops: HeaderOps<T>,
) -> Result<()> {
    let HeaderOps {
        set: set_header,
        add: add_header,
        remove: remove_header,
    } = header_ops;
    for NameValuePair { name: key, value } in &transform.set {
        if value.is_empty() {
            // This is following the classic transformation filter behavior
            remove_header(ops, key);
            continue;
        }
        let rendered = render_or_abort(env, ctx, value, value, parsed_body_as_json, errors)?;
        if let Some(rendered_str) = rendered.as_deref().filter(|s| !s.is_empty()) {
            set_header(ops, key, rendered_str.as_bytes());
        } else {
            remove_header(ops, key);
        }
    }

    for NameValuePair { name: key, value } in &transform.add {
        if value.is_empty() {
            continue;
        }
        let rendered = render_or_abort(env, ctx, value, value, parsed_body_as_json, errors)?;
        if let Some(rendered_str) = rendered.as_deref().filter(|s| !s.is_empty()) {
            add_header(ops, key, rendered_str.as_bytes());
        }
    }

    for key in &transform.remove {
        remove_header(ops, key);
    }
    Ok(())
}

/// Transform Request
///
/// On any header rendering errors, we will remove the header and continue
/// All the errors are collected and bubble up the chain so they can be logged
/// On body parsing as json error, we return error immediately so we can send a
/// 400 response back
pub fn transform_request<T: TransformationOps>(
    env: &Environment<'static>,
    transform: &LocalTransform,
    request_headers_map: &HashMap<String, String>,
    flags: ProcessFlags,
    mut ops: T,
) -> Result<()> {
    let mut m = HashMap::new();

    // for request rendering, both the header() and request_header() use the request_headers
    // so, setting both to the request_headers_map in the context
    let value = minijinja::Value::from_serialize(request_headers_map);
    m.insert(STATE_LOOKUP_KEY_HEADERS.to_string(), value.clone());
    m.insert(STATE_LOOKUP_KEY_REQ_HEADERS.to_string(), value);

    let mut parsed_body_as_json = false;
    if flags.contains(ProcessFlags::BODY) {
        if let Some(body_transform) = transform.body.as_ref() {
            if matches!(body_transform.parse_as, BodyParseBehavior::AsJson) {
                let json = ops.parse_request_json_body()?;

                parsed_body_as_json =
                    add_context_from_json_body(&mut m, json, &body_transform.value);
            }

            if body_transform.value.contains("body()") {
                let body = ops.get_request_body();
                m.insert(
                    STATE_LOOKUP_KEY_BODY.to_string(),
                    minijinja::Value::from_serialize(String::from_utf8_lossy(&body)),
                );
            }
        }
    }
    let ctx = minijinja::Value::from(m);

    let mut errors = Vec::new();
    if flags.contains(ProcessFlags::BODY) {
        if let Some(body_transform) = transform.body.as_ref() {
            if !body_transform.value.is_empty() {
                errors.extend(render_and_set_body(
                    env,
                    &ctx,
                    &body_transform.value,
                    REQUEST_BODY_TEMPLATE_LOOKUP_KEY,
                    parsed_body_as_json,
                    &mut ops,
                    BodyOps {
                        drain_body: T::drain_request_body,
                        set_header: T::set_request_header,
                        append_body: T::append_request_body,
                        remove_header: T::remove_request_header,
                    },
                ));
            }
        }
    }

    if flags.contains(ProcessFlags::HEADER) {
        process_headers(
            env,
            &ctx,
            transform,
            parsed_body_as_json,
            &mut errors,
            &mut ops,
            HeaderOps {
                set: T::set_request_header,
                add: T::add_request_header,
                remove: T::remove_request_header,
            },
        )?;
    }

    for (i, meta) in transform.dynamic_metadata.iter().enumerate() {
        if meta.namespace.is_empty() || meta.key.is_empty() || meta.value.is_empty() {
            let mut missing_fields = Vec::new();
            if meta.namespace.is_empty() {
                missing_fields.push("namespace");
            }
            if meta.key.is_empty() {
                missing_fields.push("key");
            }
            if meta.value.is_empty() {
                missing_fields.push("value");
            }
            errors.push(Error::msg(format!(
                "request.dynamicMetadata[{}] is invalid: empty {}",
                i,
                missing_fields.join(", ")
            )));
            continue;
        }
        let template_key = format!("request.dynamicMetadata[{}.{}]", meta.namespace, meta.key);
        let rendered = render_or_abort(
            env,
            &ctx,
            &template_key,
            meta.value.as_str_template().unwrap_or_default(),
            parsed_body_as_json,
            &mut errors,
        )?;
        if let Some(v) = rendered.as_deref() {
            if !v.is_empty() {
                ops.set_dynamic_metadata_string(&meta.namespace, &meta.key, v);
            }
        }
    }

    combine_errors("transform_request()", errors)
}

/// Transform Response
///
/// On any rendering errors, we will remove the header and continue
/// All the errors are collected and bubble up the chain so they can be logged
/// On body parsing as json error, we return error immediately so we can send a
/// 400 response back
pub fn transform_response<T: TransformationOps>(
    env: &Environment<'static>,
    transform: &LocalTransform,
    request_headers_map: &HashMap<String, String>,
    response_headers_map: &HashMap<String, String>,
    flags: ProcessFlags,
    mut ops: T,
) -> Result<()> {
    let mut m = HashMap::new();

    // for response rendering, header() uses response_headers and request_header()
    // uses the request_headers. So, setting them in the context accordingly
    m.insert(
        STATE_LOOKUP_KEY_HEADERS.to_string(),
        minijinja::Value::from_serialize(response_headers_map),
    );
    m.insert(
        STATE_LOOKUP_KEY_REQ_HEADERS.to_string(),
        minijinja::Value::from_serialize(request_headers_map),
    );
    let mut parsed_body_as_json = false;
    if flags.contains(ProcessFlags::BODY) {
        if let Some(body_transform) = transform.body.as_ref() {
            if matches!(body_transform.parse_as, BodyParseBehavior::AsJson) {
                let json = ops.parse_response_json_body()?;

                parsed_body_as_json =
                    add_context_from_json_body(&mut m, json, &body_transform.value);
            }

            if body_transform.value.contains("body()") {
                let body = ops.get_response_body();
                m.insert(
                    STATE_LOOKUP_KEY_BODY.to_string(),
                    minijinja::Value::from_serialize(String::from_utf8_lossy(&body)),
                );
            }
        }
    }

    let ctx = minijinja::Value::from(m);
    let mut errors = Vec::new();

    if flags.contains(ProcessFlags::BODY) {
        if let Some(body_transform) = transform.body.as_ref() {
            if !body_transform.value.is_empty() {
                errors.extend(render_and_set_body(
                    env,
                    &ctx,
                    &body_transform.value,
                    RESPONSE_BODY_TEMPLATE_LOOKUP_KEY,
                    parsed_body_as_json,
                    &mut ops,
                    BodyOps {
                        drain_body: T::drain_response_body,
                        set_header: T::set_response_header,
                        append_body: T::append_response_body,
                        remove_header: T::remove_response_header,
                    },
                ));
            }
        }
    }

    if flags.contains(ProcessFlags::HEADER) {
        process_headers(
            env,
            &ctx,
            transform,
            parsed_body_as_json,
            &mut errors,
            &mut ops,
            HeaderOps {
                set: T::set_response_header,
                add: T::add_response_header,
                remove: T::remove_response_header,
            },
        )?;
    }

    for (i, meta) in transform.dynamic_metadata.iter().enumerate() {
        if meta.namespace.is_empty() || meta.key.is_empty() || meta.value.is_empty() {
            let mut missing_fields = Vec::new();
            if meta.namespace.is_empty() {
                missing_fields.push("namespace");
            }
            if meta.key.is_empty() {
                missing_fields.push("key");
            }
            if meta.value.is_empty() {
                missing_fields.push("value");
            }
            errors.push(Error::msg(format!(
                "response.dynamicMetadata[{}] is invalid: empty {}",
                i,
                missing_fields.join(", ")
            )));
            continue;
        }
        let template_key = format!("response.dynamicMetadata[{}.{}]", meta.namespace, meta.key);
        let rendered = render_or_abort(
            env,
            &ctx,
            &template_key,
            meta.value.as_str_template().unwrap_or_default(),
            parsed_body_as_json,
            &mut errors,
        )?;
        if let Some(v) = rendered.as_deref() {
            if !v.is_empty() {
                ops.set_dynamic_metadata_string(&meta.namespace, &meta.key, v);
            }
        }
    }

    combine_errors("transform_response()", errors)
}

pub fn create_env_with_templates(
    config: &LocalTransformationConfig,
) -> Result<Environment<'static>> {
    let mut env = new_jinja_env();
    if let Some(request) = &config.request {
        for pair in &request.add {
            if pair.value.is_empty() {
                continue;
            }
            env.add_template_owned(pair.value.clone(), pair.value.clone())?;
        }
        for pair in &request.set {
            if pair.value.is_empty() {
                continue;
            }
            env.add_template_owned(pair.value.clone(), pair.value.clone())?;
        }
        if let Some(body) = &request.body {
            if !matches!(body.parse_as, BodyParseBehavior::None) && !body.value.is_empty() {
                env.add_template_owned(REQUEST_BODY_TEMPLATE_LOOKUP_KEY, body.value.clone())?;
            }
        }
        for meta in request.dynamic_metadata.iter() {
            if meta.namespace.is_empty() || meta.key.is_empty() || meta.value.is_empty() {
                continue;
            }
            env.add_template_owned(
                format!("request.dynamicMetadata[{}.{}]", meta.namespace, meta.key),
                meta.value.string_value.clone().unwrap_or_default(),
            )?;
        }
    }
    if let Some(response) = &config.response {
        for pair in &response.add {
            if pair.value.is_empty() {
                continue;
            }
            env.add_template_owned(pair.value.clone(), pair.value.clone())?;
        }
        for pair in &response.set {
            if pair.value.is_empty() {
                continue;
            }
            env.add_template_owned(pair.value.clone(), pair.value.clone())?;
        }
        if let Some(body) = &response.body {
            if !matches!(body.parse_as, BodyParseBehavior::None) && !body.value.is_empty() {
                env.add_template_owned(RESPONSE_BODY_TEMPLATE_LOOKUP_KEY, body.value.clone())?;
            }
        }
        for meta in response.dynamic_metadata.iter() {
            if meta.namespace.is_empty() || meta.key.is_empty() || meta.value.is_empty() {
                continue;
            }
            env.add_template_owned(
                format!("response.dynamicMetadata[{}.{}]", meta.namespace, meta.key),
                meta.value.string_value.clone().unwrap_or_default(),
            )?;
        }
    }
    Ok(env)
}
