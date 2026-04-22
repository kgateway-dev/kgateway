#![deny(clippy::unwrap_used, clippy::expect_used)]

use acl::{Acl, Action, DenyResponse};
use envoy_proxy_dynamic_modules_rust_sdk::*;
use std::net::{IpAddr, SocketAddr};
use std::str::FromStr;
use std::sync::Arc;

const METADATA_NAMESPACE: &str = "dev.kgateway.http.acl";
const METADATA_BLOCKED_BY_KEY: &str = "blocked-by";
const BLOCKED_BY_DEFAULT: &str = "default";
const BLOCKED_BY_UNNAMED_RULE: &str = "rule";
const BLOCKED_BY_UNKNOWN_IP: &str = "unknown-ip";
const BLOCKED_COUNTER_NAME: &str = "dev.kgateway.http.acl.blocked";

pub struct FilterConfig {
    acl: Arc<Acl>,
    blocked_counter: EnvoyCounterId,
}

impl FilterConfig {
    pub fn new<EC: EnvoyHttpFilterConfig>(envoy_filter_config: &mut EC, cfg: &str) -> Option<Self> {
        let acl = match Acl::from_json(cfg) {
            Ok(a) => a,
            Err(e) => {
                envoy_log_error!("http-acl: bad filter config: {e}");
                return None;
            }
        };
        let blocked_counter = match envoy_filter_config.define_counter(BLOCKED_COUNTER_NAME) {
            Ok(id) => id,
            Err(e) => {
                envoy_log_error!(
                    "http-acl: failed to define counter {BLOCKED_COUNTER_NAME}: {e:?}"
                );
                return None;
            }
        };
        Some(Self {
            acl: Arc::new(acl),
            blocked_counter,
        })
    }
}

impl<EHF: EnvoyHttpFilter> HttpFilterConfig<EHF> for FilterConfig {
    fn new_http_filter(&self, _envoy: &mut EHF) -> Box<dyn HttpFilter<EHF>> {
        Box::new(Filter {
            default_acl: Arc::clone(&self.acl),
            per_route_acl: None,
            blocked_counter: self.blocked_counter,
        })
    }
}

pub struct PerRouteConfig {
    pub acl: Arc<Acl>,
}

impl PerRouteConfig {
    pub fn new(cfg: &str) -> Option<Self> {
        match Acl::from_json(cfg) {
            Ok(a) => Some(Self { acl: Arc::new(a) }),
            Err(e) => {
                envoy_log_error!("http-acl: bad per-route config: {e}");
                None
            }
        }
    }
}

struct Filter {
    default_acl: Arc<Acl>,
    per_route_acl: Option<Arc<Acl>>,
    blocked_counter: EnvoyCounterId,
}

impl Filter {
    fn effective_acl(&self) -> &Acl {
        match self.per_route_acl.as_deref() {
            Some(a) => a,
            None => self.default_acl.as_ref(),
        }
    }

    fn set_per_route_config<EHF: EnvoyHttpFilter>(&mut self, envoy_filter: &mut EHF) {
        if self.per_route_acl.is_some() {
            return;
        }
        let Some(cfg) = envoy_filter.get_most_specific_route_config() else {
            return;
        };
        match cfg.downcast_ref::<PerRouteConfig>() {
            Some(prc) => self.per_route_acl = Some(Arc::clone(&prc.acl)),
            None => envoy_log_error!("http-acl: per-route config has unexpected type"),
        }
    }
}

impl<EHF: EnvoyHttpFilter> HttpFilter<EHF> for Filter {
    fn on_request_headers(
        &mut self,
        envoy_filter: &mut EHF,
        _end_of_stream: bool,
    ) -> abi::envoy_dynamic_module_type_on_http_filter_request_headers_status {
        self.set_per_route_config(envoy_filter);
        let acl = self.effective_acl();
        let ip = match extract_source_ip(envoy_filter) {
            Some(ip) => ip,
            None => {
                envoy_log_warn!("http-acl: could not determine downstream source address; denying");
                return deny(
                    envoy_filter,
                    acl.deny_response(),
                    Some(BLOCKED_BY_UNKNOWN_IP),
                    self.blocked_counter,
                );
            }
        };
        let decision = acl.evaluate(ip);
        match decision.action {
            Action::Allow => {
                abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::Continue
            }
            Action::Deny => {
                let blocked_by = if decision.default_applied {
                    BLOCKED_BY_DEFAULT
                } else {
                    decision
                        .matched_rule_name
                        .unwrap_or(BLOCKED_BY_UNNAMED_RULE)
                };
                deny(
                    envoy_filter,
                    acl.deny_response(),
                    Some(blocked_by),
                    self.blocked_counter,
                )
            }
        }
    }
}

fn extract_source_ip<EHF: EnvoyHttpFilter>(envoy_filter: &mut EHF) -> Option<IpAddr> {
    let buf = envoy_filter
        .get_attribute_string(abi::envoy_dynamic_module_type_attribute_id::SourceAddress)?;
    let s = std::str::from_utf8(buf.as_slice()).ok()?;
    // Envoy may return either a bare IP ("10.0.0.1", "::1") or "ip:port"
    // ("10.0.0.1:53124", "[::1]:53124"). Try SocketAddr first, fall back to IpAddr.
    if let Ok(sa) = SocketAddr::from_str(s) {
        return Some(sa.ip());
    }
    IpAddr::from_str(s).ok()
}

fn deny<EHF: EnvoyHttpFilter>(
    envoy_filter: &mut EHF,
    resp: &DenyResponse,
    blocked_by: Option<&str>,
    blocked_counter: EnvoyCounterId,
) -> abi::envoy_dynamic_module_type_on_http_filter_request_headers_status {
    if let Err(e) = envoy_filter.increment_counter(blocked_counter, 1) {
        envoy_log_warn!("http-acl: failed to increment blocked counter: {e:?}");
    }
    if let Some(tag) = blocked_by {
        envoy_filter.set_dynamic_metadata_string(METADATA_NAMESPACE, METADATA_BLOCKED_BY_KEY, tag);
    }
    let mut headers: Vec<(&str, &[u8])> = resp
        .headers
        .iter()
        .map(|h| (h.name.as_str(), h.value.as_bytes()))
        .collect();
    if let (Some(header_name), Some(tag)) = (resp.add_blocked_by_header.as_deref(), blocked_by) {
        headers.push((header_name, tag.as_bytes()));
    }
    envoy_filter.send_response(
        u32::from(resp.status_code),
        headers,
        None,
        Some("http-acl: denied by policy"),
    );
    abi::envoy_dynamic_module_type_on_http_filter_request_headers_status::StopIteration
}

#[cfg(test)]
mod test;
