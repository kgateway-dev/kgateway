use minijinja::value::Rest;
use minijinja::{context, Environment, State};
use serde::Deserialize;
use std::collections::HashMap;

// substring can be called with either two or three arguments --
// the first argument is the string to be modified, the second is the start position
// of the substring, and the optional third argument is the length of the substring.
// If the third argument is not provided, the substring will extend to the end of the string.
fn substring(input: &str, args: Rest<String>) -> String {
    if args.is_empty() || args.len() > 2 {
        return input.to_string();
    }
    let start: usize = args[0].parse::<usize>().unwrap_or(0);
    let end = if args.len() == 2 {
        args[1].parse::<usize>().unwrap_or(input.len())
    } else {
        input.len()
    };

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

pub fn new_jinja_env() -> Environment<'static> {
    let mut env = Environment::new();

    // could add in line like this if we wanted to
    // env.add_function("substring", |input: &str, args: Rest<String>| {

    env.add_function("substring", substring);

    // !! Standard string manipulation
    // env.add_function("trim", trim);
    // env.add_function("base64_encode", base64_encode);
    // env.add_function("base64url_encode", base64url_encode);
    // env.add_function("base64_decode", base64_decode);
    // env.add_function("base64url_decode", base64url_decode);
    // env.add_function("replace_with_random", replace_with_random);
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

pub fn transform_request_headers<F>(
    setters: &Vec<(String, String)>,
    env: &Environment<'static>,
    request_headers_map: &HashMap<String, String>,
    mut set_request_header: F,
) where
    F: FnMut(&str, &[u8]) -> bool,
{
    for (key, value) in setters {
        let tmpl = env.template_from_str(value).unwrap();
        let rendered = tmpl.render(
            context!(headers => request_headers_map, request_headers => request_headers_map),
        );
        let mut rendered_str = "".to_string();
        if let Ok(rendered_val) = rendered {
            rendered_str = rendered_val;
        } else {
            eprintln!("Error rendering template: {}", rendered.err().unwrap());
        }
        set_request_header(key, rendered_str.as_bytes());
    }
}

pub fn transform_response_headers<F>(
    setters: &Vec<(String, String)>,
    env: &Environment<'static>,
    request_headers_map: &HashMap<String, String>,
    response_headers_map: &HashMap<String, String>,
    mut set_response_header: F,
) where
    F: FnMut(&str, &[u8]) -> bool,
{
    for (key, value) in setters {
        let tmpl = env.template_from_str(value).unwrap();
        let rendered = tmpl.render(
            context!(headers => response_headers_map, request_headers => request_headers_map),
        );
        let mut rendered_str = "".to_string();
        if let Ok(rendered_val) = rendered {
            rendered_str = rendered_val;
        } else {
            eprintln!("Error rendering template: {}", rendered.err().unwrap());
        }
        set_response_header(key, rendered_str.as_bytes());
    }
}
