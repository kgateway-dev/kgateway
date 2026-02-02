/*
TODO: look into enabling this to avoid accidental use of unwrap() and
crash the process. However, there are many tests using unwrap() that
will make the linter unhappy. :w
#![deny(clippy::unwrap_used, clippy::expect_used)]
*/

use anyhow::Result;
use serde::Deserialize;
use serde_json::Value as JsonValue;

pub mod jinja;

#[derive(Clone, Deserialize)]
pub struct LocalTransformationConfig {
    #[serde(default)]
    pub request: Option<LocalTransform>,
    #[serde(default)]
    pub response: Option<LocalTransform>,
}

#[derive(Default, Clone, Deserialize)]
pub struct LocalTransform {
    #[serde(default)]
    pub add: Vec<NameValuePair>,
    #[serde(default)]
    pub set: Vec<NameValuePair>,
    #[serde(default)]
    pub remove: Vec<String>,
    #[serde(default)]
    pub body: Option<BodyTransform>,
}

impl LocalTransform {
    pub fn is_empty(&self) -> bool {
        self.add.is_empty()
            && self.set.is_empty()
            && self.remove.is_empty()
            && self.body.as_ref().map(|c| c.is_empty()).unwrap_or(true)
    }
}

#[derive(Default, Clone, Deserialize)]
pub struct BodyTransform {
    #[serde(default, rename = "parseAs")]
    pub parse_as: BodyParseBehavior,
    #[serde(default)]
    pub value: String,
}

impl BodyTransform {
    // This function is used to check if we need to do anything to the request/response at all
    // If parse_as is set to AsJson, even the value is empty (meaning we are not changing the body)
    // there is still works to do (parsing the body as json), so the json value can be used in
    // header transformation.
    // Further optimization can be done by also checking if there are any header transformation
    // at all, if not, we can return true if value is empty regardless of what parse_as is set to.
    pub fn is_empty(&self) -> bool {
        if self.value.is_empty() && matches!(self.parse_as, BodyParseBehavior::AsString) {
            return true;
        }
        false
    }
}
#[derive(Default, Clone, Deserialize)]
pub struct NameValuePair {
    pub name: String,
    #[serde(default)]
    pub value: String,
}

#[derive(Default, Clone, Deserialize)]
pub enum BodyParseBehavior {
    #[default]
    AsString,
    AsJson,
}

pub trait TransformationOps {
    fn add_request_header(&mut self, key: &str, value: &[u8]) -> bool;
    fn set_request_header(&mut self, key: &str, value: &[u8]) -> bool;
    fn remove_request_header(&mut self, key: &str) -> bool;
    fn add_response_header(&mut self, key: &str, value: &[u8]) -> bool;
    fn set_response_header(&mut self, key: &str, value: &[u8]) -> bool;
    fn remove_response_header(&mut self, key: &str) -> bool;
    fn parse_request_json_body(&mut self) -> Result<JsonValue>;
    fn get_request_body(&mut self) -> Vec<u8>;
    fn drain_request_body(&mut self, number_of_bytes: usize) -> bool;
    fn append_request_body(&mut self, data: &[u8]) -> bool;
    fn parse_response_json_body(&mut self) -> Result<JsonValue>;
    fn get_response_body(&mut self) -> Vec<u8>;
    fn drain_response_body(&mut self, number_of_bytes: usize) -> bool;
    fn append_response_body(&mut self, data: &[u8]) -> bool;
}

#[derive(thiserror::Error, Debug)]
pub enum TransformationError {
    #[error("undeclared json variables: {0}")]
    UndeclaredJsonVariables(String),
}
