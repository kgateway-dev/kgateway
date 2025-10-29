use serde::Deserialize;
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
