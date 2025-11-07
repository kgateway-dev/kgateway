use serde::de::{self, Deserializer};
use serde::Deserialize;
use serde_json::Value;
type Strng = String;

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
    #[serde(deserialize_with = "deserialize_name_value")]
    pub add: Vec<(Strng, Strng)>,
    #[serde(default)]
    #[serde(deserialize_with = "deserialize_name_value")]
    pub set: Vec<(Strng, Strng)>,
    #[serde(default)]
    pub remove: Vec<Strng>,
    #[serde(default)]
    pub body: Option<BodyTransform>,
}

#[derive(Default, Clone, Deserialize)]
pub struct BodyTransform {
    #[serde(default)]
    pub parse_as: Strng,
    #[serde(default)]
    pub value: String,
}

fn deserialize_name_value<'de, D>(deserializer: D) -> Result<Vec<(Strng, Strng)>, D::Error>
where
    D: Deserializer<'de>,
{
    let raw: Vec<Value> = Deserialize::deserialize(deserializer)?;
    let mut result = Vec::new();

    for item in raw {
        if let Some(name) = item.get("name") {
            let header_name = name.as_str().unwrap().to_string();
            let mut header_value = String::new();
            if let Some(value) = item.get("value") {
                header_value = value.as_str().unwrap().to_string();
            }
            result.push((header_name, header_value));
        } else {
            return Err(de::Error::custom("missing name in header item"));
        }
    }

    Ok(result)
}

pub trait TransformationOps {
    fn set_request_header(&mut self, key: &str, value: &[u8]) -> bool;
    fn remove_request_header(&mut self, key: &str) -> bool;
    fn set_response_header(&mut self, key: &str, value: &[u8]) -> bool;
    fn remove_response_header(&mut self, key: &str) -> bool;
}
