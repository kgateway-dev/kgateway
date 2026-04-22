#![deny(clippy::unwrap_used, clippy::expect_used)]

use ip_network::IpNetwork;
use ip_network_table_deps_treebitmap::IpLookupTable;
use serde::Deserialize;
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};
use std::str::FromStr;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Action {
    Allow,
    Deny,
}

#[derive(Debug, Clone, Deserialize)]
pub struct Rule {
    #[serde(default)]
    pub name: Option<String>,
    pub cidr: String,
    pub action: Action,
}

#[derive(Debug, Clone, Deserialize)]
pub struct Header {
    pub name: String,
    pub value: String,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DenyResponse {
    #[serde(default = "default_deny_status")]
    pub status_code: u16,
    #[serde(default)]
    pub headers: Vec<Header>,
    /// When set, the filter adds a response header with this name carrying
    /// the same value as the `blocked-by` dynamic metadata: the matched rule's
    /// `name`, or `"rule"` for an unnamed rule, or `"default"` for a
    /// default-action deny, or `"unknown-ip"` when the downstream source
    /// address cannot be determined.
    #[serde(default)]
    pub add_blocked_by_header: Option<String>,
}

fn default_deny_status() -> u16 {
    403
}

impl Default for DenyResponse {
    fn default() -> Self {
        Self {
            status_code: default_deny_status(),
            headers: Vec::new(),
            add_blocked_by_header: None,
        }
    }
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AclConfig {
    pub default_action: Action,
    #[serde(default)]
    pub rules: Vec<Rule>,
    #[serde(default)]
    pub deny_response: DenyResponse,
}

#[derive(thiserror::Error, Debug)]
pub enum AclError {
    #[error("invalid JSON: {0}")]
    Json(#[from] serde_json::Error),
    #[error("invalid CIDR `{0}`")]
    InvalidCidr(String),
}

#[derive(Debug, Clone)]
struct RuleEntry {
    action: Action,
    name: Option<String>,
}

pub struct Acl {
    default_action: Action,
    deny_response: DenyResponse,
    v4: IpLookupTable<Ipv4Addr, RuleEntry>,
    v6: IpLookupTable<Ipv6Addr, RuleEntry>,
}

/// Result of evaluating a single request against the ACL.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Decision<'a> {
    pub action: Action,
    /// Name of the rule that produced this decision. `None` when the matched
    /// rule was unnamed or when the default action was applied (check
    /// `default_applied` to distinguish those cases).
    pub matched_rule_name: Option<&'a str>,
    /// `true` when no rule matched and the configured `defaultAction` was applied.
    pub default_applied: bool,
}

impl Acl {
    pub fn from_json(s: &str) -> Result<Self, AclError> {
        let cfg: AclConfig = serde_json::from_str(s)?;
        Self::from_config(cfg)
    }

    pub fn from_config(cfg: AclConfig) -> Result<Self, AclError> {
        let mut v4: IpLookupTable<Ipv4Addr, RuleEntry> = IpLookupTable::new();
        let mut v6: IpLookupTable<Ipv6Addr, RuleEntry> = IpLookupTable::new();
        for rule in cfg.rules {
            let entry = RuleEntry {
                action: rule.action,
                name: rule.name,
            };
            match parse_network(&rule.cidr)? {
                IpNetwork::V4(n) => {
                    v4.insert(n.network_address(), u32::from(n.netmask()), entry);
                }
                IpNetwork::V6(n) => {
                    v6.insert(n.network_address(), u32::from(n.netmask()), entry);
                }
            }
        }
        Ok(Self {
            default_action: cfg.default_action,
            deny_response: cfg.deny_response,
            v4,
            v6,
        })
    }

    /// Returns the decision for the given address using longest-prefix match.
    /// IPv4-mapped IPv6 addresses (::ffff:a.b.c.d) are evaluated against the IPv4 trie.
    pub fn evaluate(&self, addr: IpAddr) -> Decision<'_> {
        let entry = match normalize(addr) {
            IpAddr::V4(v4) => self.v4.longest_match(v4).map(|(_, _, e)| e),
            IpAddr::V6(v6) => self.v6.longest_match(v6).map(|(_, _, e)| e),
        };
        match entry {
            Some(e) => Decision {
                action: e.action,
                matched_rule_name: e.name.as_deref(),
                default_applied: false,
            },
            None => Decision {
                action: self.default_action,
                matched_rule_name: None,
                default_applied: true,
            },
        }
    }

    pub fn deny_response(&self) -> &DenyResponse {
        &self.deny_response
    }
}

fn normalize(addr: IpAddr) -> IpAddr {
    if let IpAddr::V6(v6) = addr {
        if let Some(v4) = v6.to_ipv4_mapped() {
            return IpAddr::V4(v4);
        }
    }
    addr
}

fn parse_network(s: &str) -> Result<IpNetwork, AclError> {
    if s.contains('/') {
        return IpNetwork::from_str(s).map_err(|_| AclError::InvalidCidr(s.to_string()));
    }
    let ip = IpAddr::from_str(s).map_err(|_| AclError::InvalidCidr(s.to_string()))?;
    let with_prefix = match ip {
        IpAddr::V4(_) => format!("{s}/32"),
        IpAddr::V6(_) => format!("{s}/128"),
    };
    IpNetwork::from_str(&with_prefix).map_err(|_| AclError::InvalidCidr(s.to_string()))
}

#[cfg(test)]
mod test;
