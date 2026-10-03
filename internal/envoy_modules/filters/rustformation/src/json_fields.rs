use std::fmt;
use std::io::Read;

use anyhow::Result;
use serde::de::{DeserializeSeed, MapAccess, SeqAccess, Visitor};
use serde::{Deserialize, Deserializer};
use serde_json::{Map, Value};
use transformation::{BodyParseBehavior, LocalTransform};

/// Select top level fields like `{{ model }}`.
pub(super) fn requested_fields(transform: &LocalTransform) -> Option<Vec<String>> {
    let body = transform.body.as_ref()?;
    if !matches!(body.parse_as, BodyParseBehavior::AsJson)
        || !body.value.is_empty()
        || !transform.dynamic_metadata.is_empty()
    {
        return None;
    }

    let mut fields = Vec::new();
    for header in transform.set.iter().chain(&transform.add) {
        let template = header.value.as_str();
        if !["{{", "{%", "{#"].iter().any(|s| template.contains(s)) {
            continue;
        }
        let field = template.strip_prefix("{{")?.strip_suffix("}}")?.trim();
        let mut chars = field.chars();
        if !chars
            .next()
            .is_some_and(|c| c.is_ascii_alphabetic() || c == '_')
            || !chars.all(|c| c.is_ascii_alphanumeric() || c == '_')
        {
            return None;
        }
        if !fields.iter().any(|f| f == field) {
            fields.push(field.to_owned());
        }
    }
    if fields.is_empty() {
        None
    } else {
        Some(fields)
    }
}

pub(super) fn parse(mut reader: impl Read, fields: &[String]) -> Result<Value> {
    // Read Envoy's buffer slices in bulk. Parsing through std::io::Read performs
    // a reader call for every byte and copies strings into the parser's scratch
    // buffer. A slice lets discarded strings be validated without copying them.
    let mut bytes = Vec::new();
    reader.read_to_end(&mut bytes)?;
    let first = bytes
        .iter()
        .find(|b| !matches!(b, b' ' | b'\n' | b'\r' | b'\t'));
    if first != Some(&b'{') {
        // Keep the full parser's behavior for non-object roots.
        return Ok(serde_json::from_slice(&bytes)?);
    }
    let mut deserializer = serde_json::Deserializer::from_slice(&bytes);
    let value = JsonFields(fields).deserialize(&mut deserializer)?;
    // Consume the entire document, including fields after the routing field,
    // and reject trailing JSON or malformed input before forwarding.
    deserializer.end()?;
    Ok(value)
}

struct JsonFields<'a>(&'a [String]);

impl<'de> DeserializeSeed<'de> for JsonFields<'_> {
    type Value = Value;

    fn deserialize<D: Deserializer<'de>>(self, deserializer: D) -> Result<Value, D::Error> {
        deserializer.deserialize_map(self)
    }
}

impl<'de> Visitor<'de> for JsonFields<'_> {
    type Value = Value;

    fn expecting(&self, formatter: &mut fmt::Formatter) -> fmt::Result {
        formatter.write_str("a JSON object")
    }

    fn visit_map<M: MapAccess<'de>>(self, mut map: M) -> Result<Value, M::Error> {
        let mut selected = Map::new();
        while let Some(key) = map.next_key::<String>()? {
            if self.0.contains(&key) {
                // Match serde_json::Value: the last duplicate key wins, and a
                // selected field retains its original type and rendering.
                selected.insert(key, map.next_value()?);
            } else {
                map.next_value::<Discard>()?;
            }
        }
        Ok(Value::Object(selected))
    }
}

/// Validate discarded values without constructing their JSON tree. IgnoredAny's
/// skipping path does not perform all of Value's number/string/depth checks;
/// deserialize_any keeps the same validation, including inside nested values.
struct Discard;

impl<'de> Deserialize<'de> for Discard {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        deserializer.deserialize_any(Self)
    }
}

impl<'de> Visitor<'de> for Discard {
    type Value = Self;

    fn expecting(&self, formatter: &mut fmt::Formatter) -> fmt::Result {
        formatter.write_str("a JSON value")
    }

    fn visit_bool<E>(self, _: bool) -> Result<Self, E> {
        Ok(self)
    }
    fn visit_i64<E>(self, _: i64) -> Result<Self, E> {
        Ok(self)
    }
    fn visit_u64<E>(self, _: u64) -> Result<Self, E> {
        Ok(self)
    }
    fn visit_f64<E>(self, _: f64) -> Result<Self, E> {
        Ok(self)
    }
    fn visit_str<E>(self, _: &str) -> Result<Self, E> {
        Ok(self)
    }
    fn visit_unit<E>(self) -> Result<Self, E> {
        Ok(self)
    }

    fn visit_seq<S: SeqAccess<'de>>(self, mut seq: S) -> Result<Self, S::Error> {
        while seq.next_element::<Self>()?.is_some() {}
        Ok(self)
    }

    fn visit_map<M: MapAccess<'de>>(self, mut map: M) -> Result<Self, M::Error> {
        while map.next_key::<Self>()?.is_some() {
            map.next_value::<Self>()?;
        }
        Ok(self)
    }
}

#[cfg(test)]
mod tests;
