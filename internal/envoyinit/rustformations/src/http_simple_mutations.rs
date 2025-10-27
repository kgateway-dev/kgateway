use envoy_proxy_dynamic_modules_rust_sdk::*;
use minijinja::value::Rest;
use minijinja::{context, Environment, State};

#[cfg(test)]
use mockall::*;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Serialize, Deserialize, Clone)]
pub struct PerRouteConfig {
    #[serde(default)]
    request_headers_setter: Vec<(String, String)>,
    #[serde(default)]
    response_headers_setter: Vec<(String, String)>,
}

impl PerRouteConfig {
    pub fn new(config: &str) -> Option<Self> {
        let per_route_config: PerRouteConfig = match serde_json::from_str(config) {
            Ok(cfg) => cfg,
            Err(err) => {
                eprintln!("Error parsing per route config: {config} {err}");
                return None;
            }
        };
        Some(per_route_config)
    }
}

/// This implements the [`envoy_proxy_dynamic_modules_rust_sdk::HttpFilterConfig`] trait.
///
/// The trait corresponds to a Envoy filter chain configuration.

#[derive(Serialize, Deserialize)]
pub struct FilterConfig {
    #[serde(default)]
    request_headers_setter: Vec<(String, String)>,
    #[serde(default)]
    response_headers_setter: Vec<(String, String)>,
}

impl FilterConfig {
    /// This is the constructor for the [`FilterConfig`].
    ///
    /// filter_config is the filter config from the Envoy config here:
    /// https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/dynamic_modules/v3/dynamic_modules.proto#envoy-v3-api-msg-extensions-dynamic-modules-v3-dynamicmoduleconfig
    pub fn new(filter_config: &str) -> Option<Self> {
        let filter_config: FilterConfig = match serde_json::from_str(filter_config) {
            // TODO(nfuden): Handle optional configuration entries more clenaly. Currently all values are required to be present
            Ok(cfg) => cfg,
            Err(err) => {
                // TODO(nfuden): Dont panic if there is incorrect configuration
                eprintln!("Error parsing filter config: {filter_config} {err}");
                return None;
            }
        };
        Some(filter_config)
    }
}

impl<EHF: EnvoyHttpFilter> HttpFilterConfig<EHF> for FilterConfig {
    /// This is called for each new HTTP filter.
    fn new_http_filter(&mut self, _envoy: &mut EHF) -> Box<dyn HttpFilter<EHF>> {
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

        Box::new(Filter {
            request_headers_setter: self.request_headers_setter.clone(),
            // request_headers_extractions: self.request_headers_extractions.clone(),
            response_headers_setter: self.response_headers_setter.clone(),
            per_route_config: None,
            env,
        })
    }
}

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

/// This sets the request and response headers to the values specified in the filter config.
pub struct Filter {
    request_headers_setter: Vec<(String, String)>,
    // request_headers_extractions: Vec<(String, String)>,
    response_headers_setter: Vec<(String, String)>,
    per_route_config: Option<Box<PerRouteConfig>>,
    env: Environment<'static>,
}

impl Filter {
    fn set_per_route_config<EHF: EnvoyHttpFilter>(&mut self, envoy_filter: &mut EHF) {
        if self.per_route_config.is_none() {
            if let Some(per_route_config) = envoy_filter.get_most_specific_route_config().as_ref() {
                let per_route_config = match per_route_config.downcast_ref::<PerRouteConfig>() {
                    Some(cfg) => cfg,
                    None => {
                        eprintln!(
                            "set_per_route_config: wrong per route config type: {:?}",
                            per_route_config
                        );
                        return;
                    }
                };
                self.per_route_config = Some(Box::new(per_route_config.clone()));
            }
        }
    }

    fn get_per_route_config(&self) -> Option<&PerRouteConfig> {
        self.per_route_config.as_deref()
    }

    fn transform_request_headers<EHF: EnvoyHttpFilter>(&self, envoy_filter: &mut EHF) {
        let setters = match self.get_per_route_config() {
            Some(config) => &config.request_headers_setter,
            None => &self.request_headers_setter,
        };

        // TODO(nfuden): find someone who knows rust to see if we really need this Hash map for serialization
        let mut headers = HashMap::new();
        for (key, val) in envoy_filter.get_request_headers() {
            let Some(key) = std::str::from_utf8(key.as_slice()).ok() else {
                continue;
            };
            let value = std::str::from_utf8(val.as_slice()).unwrap().to_string();

            headers.insert(key.to_string(), value);
        }

        for (key, value) in setters {
            let mut env = self.env.clone();
            env.add_template("temp", value).unwrap();
            let tmpl = env.get_template("temp").unwrap();
            let rendered = tmpl.render(context!(headers => headers, request_headers => headers));
            let mut rendered_str = "".to_string();
            if let Ok(rendered_val) = rendered {
                rendered_str = rendered_val;
            } else {
                eprintln!("Error rendering template: {}", rendered.err().unwrap());
            }
            envoy_filter.set_request_header(key, rendered_str.as_bytes());
        }
    }
}

/// This implements the [`envoy_proxy_dynamic_modules_rust_sdk::HttpFilter`] trait.
impl<EHF: EnvoyHttpFilter> HttpFilter<EHF> for Filter {
    fn on_request_headers(
        &mut self,
        envoy_filter: &mut EHF,
        _end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_request_headers_status {
        // TODO: need to test if we get called even if there is no transformation setting
        //       if yes, we need to short circuit here and return Continue
        if !_end_of_stream {
            // TODO: this here always stop iteration to wait for the full request body,
            //       need to support body passthrough
            return abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::StopIteration;
        }

        self.set_per_route_config(envoy_filter);
        self.transform_request_headers(envoy_filter);
        abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue
    }

    fn on_request_body(
        &mut self,
        envoy_filter: &mut EHF,
        end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_request_body_status {
        // TODO: need to test if we get called even if there is no transformation setting
        //       if yes, we need to short circuit here and return Continue
        if !end_of_stream {
            // TODO: Technically, we don't need to buffer the body yet as we don't support parsing the body now
            //       but it will be coming next. This is mimicking the C++ transformation filter behavior to
            //       always buffer the request body by default unless passthrough is set. Will revisit and consider
            //       if this is the desired behavior when we implement parsing the body
            return abi::envoy_dynamic_module_type_on_http_filter_request_body_status::StopIterationAndBuffer;
        }

        self.set_per_route_config(envoy_filter);
        self.transform_request_headers(envoy_filter);
        abi::envoy_dynamic_module_type_on_http_filter_request_body_status::Continue
    }

    fn on_response_headers(
        &mut self,
        envoy_filter: &mut EHF,
        _end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_response_headers_status {
        // TODO(nfuden): find someone who knows rust to see if we really need this Hash map for serialization
        let mut headers = HashMap::new();
        for (key, val) in envoy_filter.get_response_headers() {
            let Some(key) = std::str::from_utf8(key.as_slice()).ok() else {
                continue;
            };
            let value = std::str::from_utf8(val.as_slice()).unwrap().to_string();

            headers.insert(key.to_string(), value);
        }

        let mut request_headers = HashMap::new();
        for (key, val) in envoy_filter.get_request_headers() {
            let Some(key) = std::str::from_utf8(key.as_slice()).ok() else {
                continue;
            };
            let value = std::str::from_utf8(val.as_slice()).unwrap().to_string();

            request_headers.insert(key.to_string(), value);
        }

        self.set_per_route_config(envoy_filter);
        let setters = match self.get_per_route_config() {
            Some(config) => &config.response_headers_setter,
            None => &self.response_headers_setter,
        };

        for (key, value) in setters {
            let mut env = self.env.clone();
            env.add_template("temp", value).unwrap();
            let tmpl = env.get_template("temp").unwrap();
            let rendered =
                tmpl.render(context!(headers => headers, request_headers => request_headers));
            let mut rendered_str = "".to_string();
            if let Ok(rendered_val) = rendered {
                rendered_str = rendered_val;
            } else {
                eprintln!("Error rendering template: {}", rendered.err().unwrap());
            }
            envoy_filter.set_response_header(key, rendered_str.as_bytes());
        }
        abi::envoy_dynamic_module_type_on_http_filter_response_headers_status::Continue
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn test_injected_functions() {
        // get envoy's mockall impl for httpfilter
        let mut envoy_filter = envoy_proxy_dynamic_modules_rust_sdk::MockEnvoyHttpFilter::default();

        // construct the filter config
        // most upstream tests start with the filter itself but we are tryign to add heavier logic
        // to the config factory strat rather than running it on header calls
        let mut filter_conf = FilterConfig {
            request_headers_setter: vec![
                (
                    "X-substring".to_string(),
                    "{{substring(\"ENVOYPROXY something\", 5, 10) }}".to_string(),
                ),
                (
                    "X-substring-no-3rd".to_string(),
                    "{{substring(\"ENVOYPROXY something\", 5) }}".to_string(),
                ),
                (
                    "X-donor-header-contents".to_string(),
                    "{{ header(\"x-donor\") }}".to_string(),
                ),
                (
                    "X-donor-header-substringed".to_string(),
                    "{{ substring( header(\"x-donor\"), 0, 7)}}".to_string(),
                ),
            ],
            response_headers_setter: vec![("X-Bar".to_string(), "foo".to_string())],
        };
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
        let mut filter_conf = FilterConfig {
            request_headers_setter: vec![(
                "X-if-truth".to_string(),
                "{%- if true -%}supersuper{% endif %}".to_string(),
            )],
            response_headers_setter: vec![("X-Bar".to_string(), "foo".to_string())],
        };
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
}
