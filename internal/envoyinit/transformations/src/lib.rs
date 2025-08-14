use serde::{Deserialize, Serialize};
use serde_with::serde_as;
type Strng = String;

pub mod jinja;

#[derive(Default, Clone, Deserialize)]
pub struct LocalTransformationConfig {
    #[serde(default)]
    pub request: Option<LocalTransform>,
    #[serde(default)]
    pub response: Option<LocalTransform>,
}

#[serde_as]
#[derive(Default, Clone, Deserialize)]
pub struct LocalTransform {
    #[serde(default)]
    #[serde_as(as = "serde_with::Map<_, _>")]
    pub add: Vec<(Strng, Strng)>,
    #[serde(default)]
    #[serde_as(as = "serde_with::Map<_, _>")]
    pub set: Vec<(Strng, Strng)>,
    #[serde(default)]
    pub remove: Vec<Strng>,
    #[serde(default)]
    pub body: Option<Strng>,
}

#[derive(Serialize, Deserialize, Clone)]
pub struct PerRouteConfig {
    #[serde(default)]
    pub request_headers_setter: Vec<(String, String)>,
    #[serde(default)]
    pub response_headers_setter: Vec<(String, String)>,
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

#[derive(Serialize, Deserialize, Clone)]
pub struct FilterConfig {
    #[serde(default)]
    pub request_headers_setter: Vec<(String, String)>,
    #[serde(default)]
    pub response_headers_setter: Vec<(String, String)>,
}

impl FilterConfig {
    /// This is the constructor for the [`FilterConfig`].
    ///
    /// filter_config is the filter config from the Envoy config here:
    /// https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/dynamic_modules/v3/dynamic_modules.proto#envoy-v3-api-msg-extensions-dynamic-modules-v3-dynamicmoduleconfig
    pub fn new(filter_config: &str) -> Option<Self> {
        let filter_config: FilterConfig = match serde_json::from_str(filter_config) {
            // TODO(nfuden): Handle optional configuration entries more cleanly. Currently all values are required to be present
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

/*
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn it_works() {
        let result = add(2, 2);
        assert_eq!(result, 4);
    }
}
*/
