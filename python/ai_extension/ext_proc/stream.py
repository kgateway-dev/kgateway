import re
import logging

from copy import copy
from logging import Logger
from typing import Iterable

from telemetry import stats
from .provider import (
    Provider,
    Tokens,
    OpenAI,
    Anthropic,
    Gemini,
    ANTHROPIC_LLM_STR,
    GEMINI_LLM_STR,
    VERTEX_AI_LLM_STR,
)

from google.protobuf import struct_pb2 as struct_pb2
from api.envoy.service.ext_proc.v3 import external_processor_pb2
from api.envoy.config.core.v3 import base_pb2 as base_pb2
from api.kgateway.policy.ai import prompt_guard
from presidio_analyzer import EntityRecognizer
from presidio_anonymizer import AnonymizerEngine
from dataclasses import dataclass, field
from openai.resources import AsyncModerations
from ext_proc.streamchunks import StreamChunks
from util.http import parse_content_type
from guardrails.regex import regex_transform

logger = logging.getLogger().getChild("kgateway-ai-ext.external_processor.handler")


@dataclass
class Info:
    body: bytearray = field(default_factory=bytearray)
    headers: dict[str, str] = field(default_factory=dict)
    content_type: str | None = None
    """
    content_type is the content type part of the mimetype. eg 'text/plain'
    This is set while we are looping through the header in set_header()
    """

    encoding: str | None = None
    """
    encoding is the charset part of the mimetype and is already lower cased. eg 'utf-8'
    This is set while we are looping through the header in set_header()
    """

    is_streaming: bool = False
    """
    is_streaming is set to true if the request is asking for a streaming response
    or the response is a streaming response base on content-type
    For req, is_streaming is set after handle_request_body.
    For resp, this is set after we handled response_headers.
    """

    def append(self, data: bytes):
        """
        Append data to the body of the Info object.
        """
        self.body += data

    def set_headers(
        self,
        header_rules: Iterable[prompt_guard.HTTPHeaderMatch],
        headers: external_processor_pb2.HttpHeaders,
    ):
        """
        This function is used to set the headers for the request and response StreamInfo objects.
        It also parse the Content-Type header and store the values in self.content_type and self.encoding

        NOTE: Right now, it only sets the headers that match the rules defined in the Webhook object.
        This is meant to save Memory, as we don't need to store all the headers in the StreamInfo object.
        If we need to store all the headers, we can filter before sending the headers to the webhook.
        """
        for header in headers.headers.headers:
            if header.key == "content-type":
                self.content_type, self.encoding = parse_content_type(
                    header.raw_value.decode("utf-8")
                )
            # Never pass through pseudo headers
            if header.key.startswith(":"):
                continue
            for rule in header_rules:
                match rule.match_type:
                    case prompt_guard.Type.EXACT:
                        if header.key == rule.key:
                            self.headers[header.key] = header.raw_value.decode("utf-8")
                    case prompt_guard.Type.REGULAR_EXPRESSION:
                        if re.match(rule.key, header.key):
                            self.headers[header.key] = header.raw_value.decode("utf-8")


@dataclass
class Handler:
    logger: Logger
    provider: Provider
    llm_provider: str
    req_webhook: prompt_guard.Webhook | None = None
    resp_webhook: prompt_guard.Webhook | None = None
    req_regex: list[EntityRecognizer] | None = None
    req_regex_action: prompt_guard.Action = prompt_guard.Action.MASK
    req_moderation: tuple[AsyncModerations, str] | None = None
    req_custom_response: prompt_guard.CustomResponse | None = None
    resp_regex: list[EntityRecognizer] | None = None
    anon: AnonymizerEngine = field(default_factory=AnonymizerEngine)
    req: Info = field(default_factory=Info)
    resp: Info = field(default_factory=Info)
    stream_chunks: StreamChunks = field(default_factory=StreamChunks)
    extra_labels: dict[str, str] = field(default_factory=dict)
    _tokens: Tokens = field(default_factory=Tokens)
    """
        Tokens for non-streaming response. streaming response token is stored 
        in stream_chunks.tokens. This member should not be accessed directly outside
        of this class, use get_tokens() instead.
        Was going to use __tokens but double underscore is not allowed for dataclass
    """

    # This value is set on the request path when we calculate the rate limited tokens.
    # It is then used on the response path to increment the rate_limited_tokens counter
    # when we've received the exact model used by the backend.
    rate_limited_tokens: int = 0
    # The request model is set on the request path, this is the model as specified by the user
    request_model: str = ""
    _response_model: str = ""
    """
    The response model is set on the response path, this is the model as returned by the provider,
    often this will be slightly more specific than the request model.
    This member is for non-streaming response, for streaming response, the model is stored in stream_chunks.model
    This should not accessed directly, use get_response_model() instead
    """

    content_encoding = ""

    _is_function_calling_response: bool = False
    """
    This boolean indicate if the response is a function calling response. For non-streaming response, 
    it's store here but for streaming response, it's stored in stream_chunks.is_function_calling
    This should not accessed directly, use is_function_calling_response() instead
    """

    def set_is_function_calling_response(self, function_calling: bool):
        """
        This function is mainly used for non-streaming response as the parsing happens outside of the handler.
        """
        self._is_function_calling_response = function_calling

    def is_function_calling_response(self) -> bool:
        if self.resp.is_streaming:
            return self.stream_chunks.is_function_calling

        return self._is_function_calling_response

    def set_response_model(self, model: str):
        """
        This function is mainly used for non-streaming response as the parsing happens outside of the handler.
        """
        self._response_model = model

    def get_response_model(self) -> str:
        if self.stream_chunks.model != "":
            return self.stream_chunks.model

        return self._response_model

    def get_tokens(self) -> Tokens:
        """ """
        if self.stream_chunks.tokens is not None:
            return copy(self.stream_chunks.tokens)

        return copy(self._tokens)

    @staticmethod
    def from_metadata(metadict: dict):
        request_id = metadict.get("x-request-id", "unknown")
        sub_logger = logger.getChild(request_id)
        llm_provider = metadict.get("x-llm-provider", "unknown")
        if llm_provider == ANTHROPIC_LLM_STR:
            handler = Handler(
                logger=sub_logger, provider=Anthropic(), llm_provider=llm_provider
            )
        elif llm_provider == GEMINI_LLM_STR:
            handler = Handler(
                logger=sub_logger, provider=Gemini(), llm_provider=llm_provider
            )
        elif llm_provider == VERTEX_AI_LLM_STR:
            handler = Handler(
                logger=sub_logger, provider=Gemini(), llm_provider=llm_provider
            )
        else:
            handler = Handler(
                logger=sub_logger, provider=OpenAI(), llm_provider=llm_provider
            )
        return handler

    def build_metadata(self) -> struct_pb2.Struct:
        tokens = self.get_tokens()
        dynamic_meta = struct_pb2.Struct(
            fields={
                "ai.kgateway.io": struct_pb2.Value(
                    struct_value=struct_pb2.Struct(
                        fields={
                            "completion_tokens": struct_pb2.Value(
                                number_value=tokens.completion
                            ),
                            "total_tokens": struct_pb2.Value(
                                number_value=tokens.total_tokens()
                            ),
                            "prompt_tokens": struct_pb2.Value(
                                number_value=tokens.prompt
                            ),
                            "rate_limited_tokens": struct_pb2.Value(
                                number_value=self.rate_limited_tokens
                            ),
                            "model": struct_pb2.Value(string_value=self.request_model),
                            "provider": struct_pb2.Value(
                                string_value=self.llm_provider
                            ),
                            "provider_model": struct_pb2.Value(
                                string_value=self.get_response_model()
                            ),
                            "streaming": struct_pb2.Value(
                                bool_value=self.resp.is_streaming
                            ),
                        }
                    )
                )
            },
        )
        return dynamic_meta

    def increment_tokens(self, jsn: dict):
        """
        This function is only used for non-streaming response.
        """
        self._tokens += self.provider.tokens(jsn)

    def build_extra_labels(self, stats_config: stats.Config, meta: base_pb2.Metadata):
        for custom_label in stats_config.custom_labels:
            self.extra_labels[custom_label.name] = custom_label.get_field(
                meta.filter_metadata.get_or_create(custom_label.metadata_namespace)
            )

    def req_regex_transform(self, role: str, content: str) -> str:
        return regex_transform(
            role, content, self.req_regex, self.anon, self.req_regex_action
        )

    def resp_regex_transform(self, role: str, content: str) -> str:
        return regex_transform(
            role,
            content,
            self.resp_regex,
            self.anon,
        )
