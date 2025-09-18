use envoy_proxy_dynamic_modules_rust_sdk::*;
use minijinja::value::Rest;
use minijinja::{context, Environment, State};

#[cfg(test)]
use mockall::*;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// This implements the [`envoy_proxy_dynamic_modules_rust_sdk::HttpFilterConfig`] trait.
///
/// The trait corresponds to a Envoy filter chain configuration.

#[derive(Serialize, Deserialize)]
pub struct FilterConfig {
    #[serde(default)]
    request_headers_setter: Vec<(String, String)>,
    #[serde(default)]
    response_headers_setter: Vec<(String, String)>,
    route_specific: HashMap<String, String>,
}

#[derive(Serialize, Deserialize, Clone)]
pub struct PerRouteConfig {
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
                eprintln!("Error parsing filter config: {err}");
                return None;
            }
        };
        Some(filter_config)
    }
}

impl<EC: EnvoyHttpFilterConfig, EHF: EnvoyHttpFilter> HttpFilterConfig<EC, EHF> for FilterConfig {
    /// This is called for each new HTTP filter.
    fn new_http_filter(&mut self, _envoy: &mut EC) -> Box<dyn HttpFilter<EHF>> {
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

        // attempt to unmarshal the route_specific strings into RouteSpecificConfigs
        // TODO(nfuden): remove this once upstream allows for real route specific configs
        let mut specific = HashMap::new();
        for (key, value) in self.route_specific.iter() {
            let route_specific: PerRouteConfig = match serde_json::from_str(value) {
                Ok(cfg) => cfg,
                Err(err) => {
                    eprintln!("Error parsing route specific config: {err} {value}");
                    continue;
                }
            };
            specific.insert(key.clone(), route_specific);
        }

        // specific.extend(self.route_specific.into_iter());

        Box::new(Filter {
            request_headers_setter: self.request_headers_setter.clone(),
            // request_headers_extractions: self.request_headers_extractions.clone(),
            response_headers_setter: self.response_headers_setter.clone(),
            // clone the hashmap
            route_specific: specific,
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
    route_specific: HashMap<String, PerRouteConfig>,
    env: Environment<'static>,
}

/// This implements the [`envoy_proxy_dynamic_modules_rust_sdk::HttpFilter`] trait.
impl<EHF: EnvoyHttpFilter> HttpFilter<EHF> for Filter {
    fn on_request_headers(
        &mut self,
        envoy_filter: &mut EHF,
        _end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_request_headers_status {
        if !_end_of_stream {
            return abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::StopIteration;
        }

        let mut setters = self.request_headers_setter.clone();
        // use the sub route version if appropriate as we dont have valid perroute config today
        if !self.route_specific.is_empty() {
            // check filter state for info
            let route_name_data_option = envoy_filter.get_metadata_string(
                abi::envoy_dynamic_module_type_metadata_source::Dynamic,
                "kgateway",
                "route",
            );
            if let Some(route_name_data) = route_name_data_option {
                // if its there then we should be able to pull the data name
                let route_name = std::str::from_utf8(route_name_data.as_slice()).unwrap();
                let route_config = self.route_specific.get(route_name);
                if let Some(route_config_val) = route_config {
                    setters = route_config_val.request_headers_setter.clone();
                }
            }
        }

        // TODO(nfuden): find someone who knows rust to see if we really need this Hash map for serialization
        let mut headers = HashMap::new();
        for (key, val) in envoy_filter.get_request_headers() {
            let Some(key) = std::str::from_utf8(key.as_slice()).ok() else {
                continue;
            };
            let value = std::str::from_utf8(val.as_slice()).unwrap().to_string();

            headers.insert(key.to_string(), value);
        }

        for (key, value) in &setters {
            let mut env = self.env.clone();
            env.add_template("temp", value).unwrap();
            let tmpl = env.get_template("temp").unwrap();
            let rendered = tmpl.render(context!(headers => headers));
            let mut rendered_str = "".to_string();
            if let Ok(rendered_val) = rendered {
                rendered_str = rendered_val;
            } else {
                eprintln!("Error rendering template: {}", rendered.err().unwrap());
            }
            envoy_filter.set_request_header(key, rendered_str.as_bytes());
        }
        abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue
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

        let mut setters = self.response_headers_setter.clone();
        // use the sub route version if appropriate as we dont have valid perroute config today
        if !self.route_specific.is_empty() {
            // check filter state for info
            let route_name_data_option = envoy_filter.get_metadata_string(
                abi::envoy_dynamic_module_type_metadata_source::Dynamic,
                "kgateway",
                "route",
            );
            if let Some(route_name_data) = route_name_data_option {
                // if its there then we should be able to pull the data name
                let route_name = std::str::from_utf8(route_name_data.as_slice()).unwrap();
                let route_config = self.route_specific.get(route_name);
                if let Some(route_config_val) = route_config {
                    setters = route_config_val.response_headers_setter.clone();
                }
            }
        }

        for (key, value) in &setters {
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
        // bootstrap envoy config to feed into the new http filter call
        struct EnvoyConfig {}
        impl EnvoyHttpFilterConfig for EnvoyConfig {}
        let mut envoy_config = EnvoyConfig {};

        // get envoy's mockall impl for httpfilter
        let mut envoy_filter = envoy_proxy_dynamic_modules_rust_sdk::MockEnvoyHttpFilter::new();

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
            route_specific: HashMap::new(),
        };
        let mut filter = filter_conf.new_http_filter(&mut envoy_config);

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
        // bootstrap envoy config to feed into the new http filter call
        struct EnvoyConfig {}
        impl EnvoyHttpFilterConfig for EnvoyConfig {}
        let mut envoy_config = EnvoyConfig {};

        // get envoy's mockall impl for httpfilter
        let mut envoy_filter = envoy_proxy_dynamic_modules_rust_sdk::MockEnvoyHttpFilter::new();

        // construct the filter config
        // most upstream tests start with the filter itself but we are tryign to add heavier logic
        // to the config factory strat rather than running it on header calls
        let mut filter_conf = FilterConfig {
            request_headers_setter: vec![(
                "X-if-truth".to_string(),
                "{%- if true -%}supersuper{% endif %}".to_string(),
            )],
            response_headers_setter: vec![("X-Bar".to_string(), "foo".to_string())],
            route_specific: HashMap::new(),
        };
        let mut filter = filter_conf.new_http_filter(&mut envoy_config);

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
