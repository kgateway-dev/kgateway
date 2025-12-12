use serde::Deserialize;

pub mod jinja;

#[derive(Default, Clone, Deserialize)]
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

#[derive(Default, Clone, Deserialize)]
pub struct BodyTransform {
    #[serde(default)]
    pub parse_as: BodyParseBehavior,
    #[serde(default)]
    pub value: String,
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
    fn set_request_header(&mut self, key: &str, value: &[u8]) -> bool;
    fn remove_request_header(&mut self, key: &str) -> bool;
    fn set_response_header(&mut self, key: &str, value: &[u8]) -> bool;
    fn remove_response_header(&mut self, key: &str) -> bool;
}
