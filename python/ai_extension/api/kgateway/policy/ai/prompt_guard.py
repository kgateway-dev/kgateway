import json
import logging
from enum import Enum
from dataclasses import dataclass, field
from typing import Optional, List
from ..ai.authtoken import SingleAuthToken, auth_token_from_json


@dataclass
class CustomResponse:
    message: Optional[str] = "The request was rejected due to inappropriate content"
    status_code: Optional[int] = 403

    @staticmethod
    def from_json(data: dict) -> "CustomResponse":
        return CustomResponse(
            message=data.get(
                "message", "The request was rejected due to inappropriate content"
            ),
            status_code=data.get("statusCode", 403),
        )


@dataclass
class RegexMatch:
    pattern: Optional[str] = None
    name: Optional[str] = None

    @staticmethod
    def from_json(data: dict) -> "RegexMatch":
        return RegexMatch(pattern=data.get("pattern"), name=data.get("name"))


class BuiltIn(Enum):
    SSN = "SSN"
    CREDIT_CARD = "CREDIT_CARD"
    PHONE_NUMBER = "PHONE_NUMBER"
    EMAIL = "EMAIL"


class Action(Enum):
    MASK = "MASK"
    REJECT = "REJECT"


@dataclass
class Regex:
    matches: Optional[List[RegexMatch]] = field(default_factory=list)
    builtins: Optional[List[BuiltIn]] = field(default_factory=list)
    action: Optional[Action] = Action.MASK  # Use Action class for default

    @staticmethod
    def from_json(data: dict) -> "Regex":
        matches = [RegexMatch.from_json(m) for m in data.get("matches", [])]
        builtins = [BuiltIn[b] for b in data.get("builtins", [])]
        return Regex(
            matches=matches,
            builtins=builtins,
            action=Action(data.get("action", "MASK")),
        )


@dataclass
class Endpoint:
    host: str
    port: int

    @staticmethod
    def from_json(data: dict) -> "Endpoint":
        return Endpoint(host=data.get("host", ""), port=data.get("port", 0))


class Type(Enum):
    EXACT = "Exact"
    REGULAR_EXPRESSION = "RegularExpression"


@dataclass
class HTTPHeaderMatch:
    name: str
    value: str
    type: Optional[Type] = Type.EXACT

    @staticmethod
    def from_json(data: dict) -> "HTTPHeaderMatch":
        return HTTPHeaderMatch(
            type=Type(data.get("type", "Exact")), name=data["name"], value=data["value"]
        )


@dataclass
class Webhook:
    endpoint: Endpoint
    forwardHeaders: Optional[List[HTTPHeaderMatch]] = field(default_factory=list)

    @staticmethod
    def from_json(data: dict) -> "Webhook":
        endpoint = Endpoint.from_json(data["host"])
        forward_headers = [
            HTTPHeaderMatch.from_json(h) for h in data.get("forwardHeaders", [])
        ]
        return Webhook(endpoint=endpoint, forwardHeaders=forward_headers)


@dataclass
class OpenAIModeration:
    model: Optional[str] = None
    auth_token: Optional[SingleAuthToken] = None

    @staticmethod
    def from_json(data: dict) -> "OpenAIModeration":
        auth_token_data = data.get("authToken")
        auth_token = None
        if auth_token_data:
            auth_token = auth_token_from_json(auth_token_data)

        return OpenAIModeration(
            model=data.get("model"),
            auth_token=auth_token,
        )


@dataclass
class Moderation:
    openai: Optional[OpenAIModeration] = None

    @staticmethod
    def from_json(data: dict) -> "Moderation":
        openai_data = data.get("openAIModeration")
        if openai_data:
            return Moderation(openai=OpenAIModeration.from_json(openai_data))

        logging.error(f"Unknown moderation type: {data}")
        return Moderation()


@dataclass
class PromptguardRequest:
    custom_response: Optional[CustomResponse] = None
    regex: Optional[Regex] = None
    webhook: Optional[Webhook] = None
    moderation: Optional[Moderation] = None


@dataclass
class PromptguardResponse:
    regex: Optional[Regex] = None
    webhook: Optional[Webhook] = None


def resp_from_json(data: str) -> PromptguardResponse:
    logging.debug(f"{data}")
    response_data = json.loads(data)

    regex_data = response_data.get("regex")
    regex = None
    if regex_data:
        regex = Regex.from_json(regex_data)

    webhook_data = response_data.get("webhook")
    webhook = None
    if webhook_data:
        webhook = Webhook.from_json(webhook_data)

    return PromptguardResponse(
        regex=regex,
        webhook=webhook,
    )


def req_from_json(data: str) -> PromptguardRequest:
    request_data = json.loads(data)

    webhook_data = request_data.get("webhook")
    webhook = None
    if webhook_data:
        webhook = Webhook.from_json(webhook_data)

    moderation_data = request_data.get("moderation")
    moderation = None
    if moderation_data:
        moderation = Moderation.from_json(moderation_data)

    custom_response_data = request_data.get("customResponse")
    custom_response = None
    if custom_response_data:
        custom_response = CustomResponse.from_json(custom_response_data)

    regex_data = request_data.get("regex")
    regex = None
    if regex_data:
        regex = Regex.from_json(regex_data)

    return PromptguardRequest(
        custom_response=custom_response,
        regex=regex,
        webhook=webhook,
        moderation=moderation,
    )
