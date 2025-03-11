import json
import logging

from google.protobuf import struct_pb2 as struct_pb2
from .utils import read_mounted_config_map

from pydantic import BaseModel, Field

logger: logging.Logger = logging.getLogger().getChild(
    "kgateway-ai-ext.external_processor"
)


class CustomLabel(BaseModel):
    """
    Represents a custom label that can be added to the stats.

    Using pydantic.BaseModel allows for much simpler Json parsing and validation.
    """

    name: str = Field(alias="name")
    metadata_namespace: str = Field(
        alias="metadataNamespace", default="envoy.filters.http.jwt_authn"
    )
    metadata_key: str = Field(alias="metadataKey")
    key_delimiter: str = Field(alias="keyDelimiter", default=":")

    def get_field(self, data: struct_pb2.Struct) -> str:
        """get_field retrieves the value for the label from the metadata struct value."""
        split = self.metadata_key.split(self.key_delimiter)
        for idx, key in enumerate(split):
            if key in data.fields:
                value = data.fields[key]
                match value.WhichOneof("kind"):
                    # If end use, otherwise keep going
                    case "string_value":
                        if idx == len(split) - 1:
                            return value.string_value
                    case "number_value":
                        if idx == len(split) - 1:
                            return str(value.number_value)
                    case "bool_value":
                        if idx == len(split) - 1:
                            return str(value.bool_value)
                    case "struct_value":
                        data = value.struct_value
                    case _:
                        return "<unknown>"
        return "<unknown>"


class Config(BaseModel):
    """
    Python representation of the AI Stats config struct from the Gateway Parameters API.

    https://github.com/kgateway-dev/kgateway/blob/75dca1e66e894325ee1b57db04c0455432228dcf/api/v1alpha1/gateway_parameters_types.go#L668
    """

    custom_labels: list[CustomLabel] = Field(alias="customLabels")

    @classmethod
    def from_file(cls, file_path: str):
        try:
            data = read_mounted_config_map(file_path)
            return cls(**data)
        except FileNotFoundError as e:
            logger.warning(f"Error: stats file config was not found: {e}")
            return cls(customLabels=[])
        except json.JSONDecodeError as e:
            logger.warning(f"Error: stats file config is not a valid JSON: {e}")
            return cls(customLabels=[])
        except Exception as ex:
            logger.warning(f"An error occurred while reading the file. {ex}")
            return cls(customLabels=[])
