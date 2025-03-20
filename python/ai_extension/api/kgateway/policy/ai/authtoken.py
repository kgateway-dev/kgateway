import logging
from enum import Enum
from dataclasses import dataclass
from typing import Optional


class SingleAuthTokenKind(Enum):
    INLINE = "Inline"
    PASSTHROUGH = "Passthrough"
    UNKNOWN = "Unknown"


@dataclass
class LocalObjectReference:
    name: Optional[str] = None

    @staticmethod
    def from_json(data: dict) -> "LocalObjectReference":
        return LocalObjectReference(**data)


@dataclass
class SingleAuthToken:
    kind: SingleAuthTokenKind
    inline: Optional[str] = None

    def __repr__(self):
        return f"SingleAuthToken(kind={self.kind}, inline={self.inline})"


def auth_token_from_json(json_data: dict) -> SingleAuthToken:
    logging.error(f"json data: {json_data}")
    if json_data.get("kind") == "Inline":
        return SingleAuthToken(
            kind=SingleAuthTokenKind.INLINE,
            inline=json_data.get("inline"),
        )
    elif json_data.get("kind") == "Passthrough":
        return SingleAuthToken(
            kind=SingleAuthTokenKind.PASSTHROUGH,
        )

    logging.error(f"Unknown auth token kind: {json_data.get('kind')}")
    return SingleAuthToken(kind=SingleAuthTokenKind.UNKNOWN)
